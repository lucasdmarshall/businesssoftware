// Pure transforms that turn the server sync/pull change feed into local cache
// rows. Kept free of any Tauri/SQLite import so they can be unit tested in a
// plain Node environment.
import type { LocalLeave, LocalShift, LocalTask } from "./localDb";

export type RemoteOperation = {
  id: string;
  entity: string;
  action: string;
  payload: Record<string, unknown>;
  created_at: string;
};

const text = (payload: Record<string, unknown>, key: string, fallback = ""): string => {
  const value = payload[key];
  return typeof value === "string" ? value : fallback;
};

const nullableText = (payload: Record<string, unknown>, key: string): string | null => {
  const value = payload[key];
  return typeof value === "string" && value !== "" ? value : null;
};

export function mapRemoteTask(operation: RemoteOperation): LocalTask {
  const payload = operation.payload ?? {};
  return {
    id: text(payload, "id"),
    title: text(payload, "title"),
    description: text(payload, "description"),
    status: text(payload, "status", "todo"),
    priority: text(payload, "priority", "normal"),
    dueAt: nullableText(payload, "due_at"),
    assignedTo: nullableText(payload, "assigned_to"),
    recurrenceRule: nullableText(payload, "recurrence_rule"),
    recurrenceNextAt: nullableText(payload, "recurrence_next_at"),
    createdAt: operation.created_at,
    updatedAt: operation.created_at,
  };
}

export function mapRemoteLeave(operation: RemoteOperation): LocalLeave {
  const payload = operation.payload ?? {};
  return {
    id: text(payload, "id"),
    requested_by: "remote",
    leave_type: text(payload, "leave_type", "annual"),
    start_date: text(payload, "start_date"),
    end_date: text(payload, "end_date"),
    reason: text(payload, "reason"),
    status: "pending",
  };
}

export function mapRemoteShift(operation: RemoteOperation): LocalShift {
  const payload = operation.payload ?? {};
  return {
    id: text(payload, "id"),
    assigned_to: text(payload, "assigned_to", "remote"),
    title: text(payload, "title"),
    shift_date: text(payload, "shift_date"),
    starts_at: text(payload, "starts_at"),
    ends_at: text(payload, "ends_at"),
    status: "scheduled",
    note: text(payload, "note"),
  };
}

// hasId guards against partial payloads that would create empty cache rows.
export const hasId = (operation: RemoteOperation): boolean => text(operation.payload ?? {}, "id") !== "";
