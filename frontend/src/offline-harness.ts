// Pure device-offline harness: in-memory outbox + cache rules that mirror
// localDb sync behavior without Tauri/SQLite. Used by automated tests and by
// production helpers that must stay free of native dependencies.
import type { LocalAttendance, LocalLeave, LocalShift, LocalTask, OutboxOperation } from "./localDb";
import { hasId, mapRemoteLeave, mapRemoteShift, mapRemoteTask, type RemoteOperation } from "./sync-transform";

export type OutboxStatus = NonNullable<OutboxOperation["status"]>;

export type PushResult = {
  accepted?: string[];
  duplicates?: string[];
  rejected?: string[];
  conflicts?: string[];
};

export type DeviceCache = {
  tasks: Map<string, LocalTask>;
  leave: Map<string, LocalLeave>;
  shifts: Map<string, LocalShift>;
  /** Attendance keyed by work_date so check-in and check-out from different devices merge. */
  attendanceByDate: Map<string, LocalAttendance>;
};

export function emptyCache(): DeviceCache {
  return {
    tasks: new Map(),
    leave: new Map(),
    shifts: new Map(),
    attendanceByDate: new Map(),
  };
}

/** Operations the device may queue while unreachable. */
export function isOfflineCapable(entity: string, action: string): boolean {
  if (entity === "task" && (action === "create" || action === "update" || action === "delete")) return true;
  if (entity === "attendance" && (action === "check_in" || action === "check_out")) return true;
  if (entity === "leave" && action === "create") return true;
  if (entity === "shift" && action === "create") return true;
  if (entity === "calendar" && action === "create") return true;
  return false;
}

/** Actions that must never be applied from the local outbox. */
export function isServerAuthoritative(entity: string, action: string): boolean {
  if (entity === "leave" && (action === "approve" || action === "reject")) return true;
  if (entity === "workflow" && (action === "approve" || action === "reject" || action === "delegate")) return true;
  if (entity === "finance" && (action === "approve" || action === "pay" || action === "confirm_payroll")) return true;
  if (entity === "rbac" || entity === "permission" || entity === "role") return true;
  if (entity === "auth" || entity === "mfa" || entity === "password") return true;
  if (entity === "user" && (action === "offboard" || action === "delete")) return true;
  return !isOfflineCapable(entity, action) && action !== "read";
}

/**
 * Maps a push response onto outbox statuses. Accepted and duplicates both count
 * as synced (idempotent replay). Conflicts stay open for human review.
 */
export function applyPushResponse(pendingIds: string[], result: PushResult): Map<string, OutboxStatus> {
  const next = new Map<string, OutboxStatus>();
  for (const id of pendingIds) next.set(id, "pending");
  for (const id of result.accepted ?? []) next.set(id, "synced");
  for (const id of result.duplicates ?? []) next.set(id, "synced");
  for (const id of result.rejected ?? []) next.set(id, "failed");
  for (const id of result.conflicts ?? []) next.set(id, "conflict");
  return next;
}

export function countPushOutcomes(statuses: Map<string, OutboxStatus>): { synced: number; failed: number; conflicts: number; pending: number } {
  let synced = 0;
  let failed = 0;
  let conflicts = 0;
  let pending = 0;
  for (const status of statuses.values()) {
    if (status === "synced") synced += 1;
    else if (status === "failed") failed += 1;
    else if (status === "conflict") conflicts += 1;
    else pending += 1;
  }
  return { synced, failed, conflicts, pending };
}

/** Applies a change-feed pull into an in-memory cache (tombstones drop tasks). */
export function applyPullOperations(cache: DeviceCache, operations: RemoteOperation[]): DeviceCache {
  const next: DeviceCache = {
    tasks: new Map(cache.tasks),
    leave: new Map(cache.leave),
    shifts: new Map(cache.shifts),
    attendanceByDate: new Map(cache.attendanceByDate),
  };

  for (const operation of operations) {
    if (!hasId(operation) && operation.action !== "delete") continue;

    if (operation.action === "delete") {
      const deletedId = operation.payload?.id;
      if (operation.entity === "task" && typeof deletedId === "string") {
        next.tasks.delete(deletedId);
      }
      continue;
    }

    if (operation.entity === "task") {
      const task = mapRemoteTask(operation);
      if (task.id) next.tasks.set(task.id, task);
    } else if (operation.entity === "leave") {
      const leave = mapRemoteLeave(operation);
      if (leave.id) next.leave.set(leave.id, leave);
    } else if (operation.entity === "shift") {
      const shift = mapRemoteShift(operation);
      if (shift.id) next.shifts.set(shift.id, shift);
    } else if (operation.entity === "attendance") {
      mergeAttendance(next.attendanceByDate, operation);
    }
  }
  return next;
}

function mergeAttendance(byDate: Map<string, LocalAttendance>, operation: RemoteOperation) {
  const payload = operation.payload ?? {};
  const workDate = typeof payload.work_date === "string" ? payload.work_date : "";
  if (!workDate) return;
  const id = typeof payload.id === "string" && payload.id !== "" ? payload.id : workDate;
  const at = typeof payload.at === "string" ? payload.at : null;
  const existing = byDate.get(workDate) ?? {
    id,
    user_id: "remote",
    work_date: workDate,
    check_in_at: null,
    check_out_at: null,
    status: "present",
    note: typeof payload.note === "string" ? payload.note : "",
  };
  if (operation.action === "check_out") {
    existing.check_out_at = at;
  } else {
    existing.check_in_at = at;
  }
  if (typeof payload.note === "string") existing.note = payload.note;
  byDate.set(workDate, existing);
}

/** Queues an offline mutation after validating the entity/action policy. */
export function queueOfflineMutation(
  outbox: OutboxOperation[],
  operation: OutboxOperation,
): { outbox: OutboxOperation[]; ok: boolean; reason?: string } {
  if (!operation.id || !operation.entity || !operation.action) {
    return { outbox, ok: false, reason: "incomplete operation" };
  }
  if (isServerAuthoritative(operation.entity, operation.action)) {
    return { outbox, ok: false, reason: "server-authoritative" };
  }
  if (!isOfflineCapable(operation.entity, operation.action)) {
    return { outbox, ok: false, reason: "not offline-capable" };
  }
  if (outbox.some((item) => item.id === operation.id)) {
    return { outbox, ok: false, reason: "duplicate operation id" };
  }
  return {
    outbox: [...outbox, { ...operation, status: "pending" }],
    ok: true,
  };
}
