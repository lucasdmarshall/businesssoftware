import Database from "@tauri-apps/plugin-sql";
import { hasId, mapRemoteLeave, mapRemoteShift, mapRemoteTask, type RemoteOperation } from "./sync-transform";

export type LocalSession = {
  email: string;
  displayName: string;
  organization: string;
  role: string;
  cachedAt: string;
  permissions: string[];
};

export type OutboxOperation = {
  id: string;
  entity: string;
  action: string;
  payload: unknown;
  createdAt: string;
  status?: "pending" | "synced" | "failed" | "conflict";
};

export type LocalTask = {
  id: string;
  title: string;
  description: string;
  status: string;
  priority: string;
  createdAt: string;
  updatedAt: string;
  assignedTo?: string | null;
  dueAt?: string | null;
  recurrenceRule?: string | null;
  recurrenceNextAt?: string | null;
};

export type LocalAttendance = {
  id: string;
  user_id: string;
  work_date: string;
  check_in_at: string | null;
  check_out_at: string | null;
  status: string;
  note: string;
};

export type LocalLeave = {
  id: string;
  requested_by: string;
  leave_type: string;
  start_date: string;
  end_date: string;
  reason: string;
  status: string;
};
export type LocalShift = { id: string; assigned_to: string; display_name?: string; title: string; shift_date: string; starts_at: string; ends_at: string; status: string; note: string };

let databasePromise: ReturnType<typeof Database.load> | null = null;

export async function localDb() {
  if (!databasePromise) databasePromise = Database.load("sqlite:name-local.db");
  return databasePromise;
}

export async function initializeLocalDb() {
  const db = await localDb();
  await db.execute(`CREATE TABLE IF NOT EXISTS local_session (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    email TEXT NOT NULL,
    display_name TEXT NOT NULL,
    organization TEXT NOT NULL,
    role TEXT NOT NULL,
    cached_at TEXT NOT NULL,
    permissions TEXT NOT NULL DEFAULT '[]'
  )`);
  try { await db.execute(`ALTER TABLE local_session ADD COLUMN permissions TEXT NOT NULL DEFAULT '[]'`); } catch { /* existing database already migrated */ }
  await db.execute(`CREATE TABLE IF NOT EXISTS sync_outbox (
    id TEXT PRIMARY KEY,
    operation TEXT NOT NULL,
    payload TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    created_at TEXT NOT NULL
  )`);
  await db.execute(`CREATE TABLE IF NOT EXISTS local_tasks (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    priority TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    assigned_to TEXT,
    due_at TEXT
  )`);
  try { await db.execute(`ALTER TABLE local_tasks ADD COLUMN assigned_to TEXT`); } catch { /* already migrated */ }
  try { await db.execute(`ALTER TABLE local_tasks ADD COLUMN due_at TEXT`); } catch { /* already migrated */ }
  try { await db.execute(`ALTER TABLE local_tasks ADD COLUMN recurrence_rule TEXT`); } catch { /* already migrated */ }
  try { await db.execute(`ALTER TABLE local_tasks ADD COLUMN recurrence_next_at TEXT`); } catch { /* already migrated */ }
  await db.execute(`CREATE TABLE IF NOT EXISTS local_attendance (
    id TEXT PRIMARY KEY, user_id TEXT NOT NULL, work_date TEXT NOT NULL,
    check_in_at TEXT, check_out_at TEXT, status TEXT NOT NULL, note TEXT NOT NULL DEFAULT ''
  )`);
  await db.execute(`CREATE TABLE IF NOT EXISTS local_leave (
    id TEXT PRIMARY KEY, requested_by TEXT NOT NULL, leave_type TEXT NOT NULL,
    start_date TEXT NOT NULL, end_date TEXT NOT NULL, reason TEXT NOT NULL DEFAULT '', status TEXT NOT NULL
  )`);
  await db.execute(`CREATE TABLE IF NOT EXISTS local_shifts (id TEXT PRIMARY KEY, assigned_to TEXT NOT NULL, title TEXT NOT NULL, shift_date TEXT NOT NULL, starts_at TEXT NOT NULL, ends_at TEXT NOT NULL, status TEXT NOT NULL, note TEXT NOT NULL DEFAULT '')`);
}

export async function cacheAttendance(records: LocalAttendance[]) {
  const db = await localDb();
  for (const record of records) await db.execute(`INSERT INTO local_attendance (id, user_id, work_date, check_in_at, check_out_at, status, note) VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(id) DO UPDATE SET check_in_at=excluded.check_in_at, check_out_at=excluded.check_out_at, status=excluded.status, note=excluded.note`, [record.id, record.user_id, record.work_date, record.check_in_at, record.check_out_at, record.status, record.note]);
}

export async function getLocalAttendance(): Promise<LocalAttendance[]> {
  const db = await localDb();
  return db.select<LocalAttendance[]>(`SELECT id, user_id, work_date, check_in_at, check_out_at, status, note FROM local_attendance ORDER BY work_date DESC LIMIT 180`);
}

export async function cacheLeave(requests: LocalLeave[]) {
  const db = await localDb();
  for (const request of requests) await db.execute(`INSERT INTO local_leave (id, requested_by, leave_type, start_date, end_date, reason, status) VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(id) DO UPDATE SET leave_type=excluded.leave_type, start_date=excluded.start_date, end_date=excluded.end_date, reason=excluded.reason, status=excluded.status`, [request.id, request.requested_by, request.leave_type, request.start_date, request.end_date, request.reason, request.status]);
}

export async function getLocalLeave(): Promise<LocalLeave[]> {
  const db = await localDb();
  return db.select<LocalLeave[]>(`SELECT id, requested_by, leave_type, start_date, end_date, reason, status FROM local_leave ORDER BY start_date DESC LIMIT 500`);
}

export async function cacheShifts(shifts: LocalShift[]) { const db = await localDb(); for (const shift of shifts) await db.execute(`INSERT INTO local_shifts (id,assigned_to,title,shift_date,starts_at,ends_at,status,note) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(id) DO UPDATE SET title=excluded.title,shift_date=excluded.shift_date,starts_at=excluded.starts_at,ends_at=excluded.ends_at,status=excluded.status,note=excluded.note`, [shift.id,shift.assigned_to,shift.title,shift.shift_date,shift.starts_at,shift.ends_at,shift.status,shift.note]); }
export async function getLocalShifts(): Promise<LocalShift[]> { const db = await localDb(); return db.select<LocalShift[]>(`SELECT id,assigned_to,title,shift_date,starts_at,ends_at,status,note FROM local_shifts ORDER BY shift_date, starts_at LIMIT 1000`); }

export async function cacheTasks(tasks: LocalTask[]) {
  const db = await localDb();
  for (const task of tasks) {
    await db.execute(`INSERT INTO local_tasks (id, title, description, status, priority, created_at, updated_at, assigned_to, due_at, recurrence_rule, recurrence_next_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
      ON CONFLICT(id) DO UPDATE SET title=excluded.title, description=excluded.description, status=excluded.status, priority=excluded.priority, updated_at=excluded.updated_at, assigned_to=excluded.assigned_to, due_at=excluded.due_at, recurrence_rule=excluded.recurrence_rule, recurrence_next_at=excluded.recurrence_next_at`,
      [task.id, task.title, task.description, task.status, task.priority, task.createdAt, task.updatedAt, task.assignedTo ?? null, task.dueAt ?? null, task.recurrenceRule ?? null, task.recurrenceNextAt ?? null]);
  }
}

export async function deleteLocalTask(id: string) {
  const db = await localDb();
  await db.execute(`DELETE FROM local_tasks WHERE id=$1`, [id]);
}

export async function getLocalTasks(): Promise<LocalTask[]> {
  const db = await localDb();
  return db.select<LocalTask[]>(`SELECT id, title, description, status, priority, created_at as createdAt, updated_at as updatedAt, assigned_to as assignedTo, due_at as dueAt, recurrence_rule as recurrenceRule, recurrence_next_at as recurrenceNextAt FROM local_tasks ORDER BY updated_at DESC`);
}

export async function cacheSession(session: LocalSession) {
  const db = await localDb();
  await db.execute(`INSERT INTO local_session (id, email, display_name, organization, role, cached_at, permissions)
    VALUES (1, $1, $2, $3, $4, $5, $6)
    ON CONFLICT(id) DO UPDATE SET email = excluded.email, display_name = excluded.display_name,
    organization = excluded.organization, role = excluded.role, cached_at = excluded.cached_at, permissions = excluded.permissions`,
    [session.email, session.displayName, session.organization, session.role, session.cachedAt, JSON.stringify(session.permissions ?? [])]);
}

export async function getCachedSession(): Promise<LocalSession | null> {
  const db = await localDb();
  const rows = await db.select<Array<Omit<LocalSession, "permissions"> & { permissions: string }>>(`SELECT email, display_name as displayName, organization, role, cached_at as cachedAt, permissions FROM local_session WHERE id = 1`);
  if (!rows[0]) return null;
  return { ...rows[0], permissions: JSON.parse(rows[0].permissions || "[]") };
}

export async function queueOperation(operation: OutboxOperation) {
  const db = await localDb();
  await db.execute(`INSERT INTO sync_outbox (id, operation, payload, status, created_at) VALUES ($1, $2, $3, 'pending', $4)`, [operation.id, `${operation.entity}.${operation.action}`, JSON.stringify(operation.payload), operation.createdAt]);
}

export async function getPendingOperations(): Promise<OutboxOperation[]> {
  const db = await localDb();
  const rows = await db.select<Array<{ id: string; operation: string; payload: string; created_at: string }>>(`SELECT id, operation, payload, created_at FROM sync_outbox WHERE status = 'pending' ORDER BY created_at ASC LIMIT 500`);
  return rows.map((row) => {
    const [entity, action] = row.operation.split(".");
    return { id: row.id, entity, action, payload: JSON.parse(row.payload), createdAt: row.created_at, status: "pending" };
  });
}

export async function getOutboxStatuses(ids: string[]) {
  if (!ids.length) return new Map<string, string>();
  const db = await localDb();
  const rows = await db.select<Array<{ id: string; status: string }>>(`SELECT id, status FROM sync_outbox WHERE id IN (${ids.map((_, index) => `$${index + 1}`).join(", ")})`, ids);
  return new Map(rows.map((row) => [row.id, row.status]));
}

export async function markOperationsSynced(ids: string[]) {
  if (!ids.length) return;
  const db = await localDb();
  for (const id of ids) await db.execute(`UPDATE sync_outbox SET status = 'synced' WHERE id = $1`, [id]);
}

export async function markOperationsFailed(ids: string[]) {
  if (!ids.length) return;
  const db = await localDb();
  for (const id of ids) await db.execute(`UPDATE sync_outbox SET status = 'failed' WHERE id = $1`, [id]);
}

export async function markOperationsConflict(ids: string[]) {
  if (!ids.length) return;
  const db = await localDb();
  for (const id of ids) await db.execute(`UPDATE sync_outbox SET status = 'conflict' WHERE id = $1`, [id]);
}

export async function discardOperation(id: string) {
  const db = await localDb();
  await db.execute(`UPDATE sync_outbox SET status = 'synced' WHERE id = $1`, [id]);
}

export async function retryOperation(id: string) {
  const db = await localDb();
  await db.execute(`UPDATE sync_outbox SET status = 'pending' WHERE id = $1`, [id]);
}

// pullRemoteChanges consumes the server change feed and writes it into the
// local SQLite caches so a device that later goes offline still sees changes
// that other clients synced. PostgreSQL remains authoritative; this only keeps
// the read-side caches current. The payload → cache-row transforms live in
// sync-transform.ts so they can be unit tested without Tauri/SQLite.
export async function pullRemoteChanges(apiBase: string) {
  const response = await fetch(`${apiBase}/sync/pull`, { credentials: "include" });
  if (!response.ok) throw new Error("pull failed");
  const result = (await response.json()) as { operations: RemoteOperation[] };
  const operations = result.operations ?? [];
  const tasks: LocalTask[] = [];
  const leave: LocalLeave[] = [];
  const shifts: LocalShift[] = [];
  const db = await localDb();
  for (const operation of operations) {
    if (!hasId(operation)) continue;
    if (operation.action === "delete") {
      // A tombstone from the change feed: drop the local copy.
      const deletedId = (operation.payload ?? {}).id;
      if (operation.entity === "task" && typeof deletedId === "string") {
        await db.execute(`DELETE FROM local_tasks WHERE id=$1`, [deletedId]);
      }
      continue;
    }
    if (operation.entity === "task") {
      tasks.push(mapRemoteTask(operation));
    } else if (operation.entity === "leave") {
      leave.push(mapRemoteLeave(operation));
    } else if (operation.entity === "shift") {
      shifts.push(mapRemoteShift(operation));
    } else if (operation.entity === "attendance") {
      // Check-in and check-out arrive as separate operations for the same
      // record, so update only the column this operation carries.
      const payload = operation.payload ?? {};
      const column = operation.action === "check_out" ? "check_out_at" : "check_in_at";
      const at = typeof payload.at === "string" ? payload.at : null;
      await db.execute(
        `INSERT INTO local_attendance (id, user_id, work_date, check_in_at, check_out_at, status, note)
         VALUES ($1, 'remote', $2, $3, $4, 'present', $5)
         ON CONFLICT(id) DO UPDATE SET ${column}=$6, note=excluded.note`,
        [payload.id, typeof payload.work_date === "string" ? payload.work_date : "", operation.action === "check_in" ? at : null, operation.action === "check_out" ? at : null, typeof payload.note === "string" ? payload.note : "", at],
      );
    }
  }
  if (tasks.length) await cacheTasks(tasks);
  if (leave.length) await cacheLeave(leave);
  if (shifts.length) await cacheShifts(shifts);
  return { applied: operations.length };
}

export async function syncPendingOperations(apiBase: string) {
  const operations = await getPendingOperations();
  if (!operations.length) return { synced: 0 };
  const response = await fetch(`${apiBase}/sync/push`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ operations }),
  });
  if (!response.ok) throw new Error("sync failed");
  const result = await response.json() as { accepted: string[]; duplicates: string[]; rejected?: string[]; conflicts?: string[] };
  const synced = [...(result.accepted ?? []), ...(result.duplicates ?? [])];
  await markOperationsSynced(synced);
  await markOperationsFailed(result.rejected ?? []);
  await markOperationsConflict(result.conflicts ?? []);
  return { synced: synced.length, failed: (result.rejected ?? []).length, conflicts: (result.conflicts ?? []).length };
}
