import { describe, expect, it } from "vitest";
import { hasId, mapRemoteLeave, mapRemoteShift, mapRemoteTask, type RemoteOperation } from "./sync-transform";

const operation = (entity: string, payload: Record<string, unknown>): RemoteOperation => ({ id: "op-1", entity, action: "create", payload, created_at: "2026-08-20T00:00:00Z" });

describe("mapRemoteTask", () => {
  it("maps a full task payload and carries the operation timestamp", () => {
    const task = mapRemoteTask(operation("task", { id: "t1", title: "Ship", description: "d", status: "in_progress", priority: "high", due_at: "2026-09-01T00:00:00Z", assigned_to: "u1", recurrence_rule: "weekly", recurrence_next_at: "2026-09-08T00:00:00Z" }));
    expect(task).toMatchObject({ id: "t1", title: "Ship", status: "in_progress", priority: "high", dueAt: "2026-09-01T00:00:00Z", assignedTo: "u1", recurrenceRule: "weekly" });
    expect(task.createdAt).toBe("2026-08-20T00:00:00Z");
    expect(task.updatedAt).toBe("2026-08-20T00:00:00Z");
  });

  it("falls back to defaults and nulls for a sparse payload", () => {
    const task = mapRemoteTask(operation("task", { id: "t2", title: "Bare" }));
    expect(task).toMatchObject({ status: "todo", priority: "normal", dueAt: null, assignedTo: null, recurrenceRule: null, recurrenceNextAt: null });
  });

  it("treats empty strings as null for optional fields", () => {
    const task = mapRemoteTask(operation("task", { id: "t3", title: "X", assigned_to: "" }));
    expect(task.assignedTo).toBeNull();
  });
});

describe("mapRemoteLeave and mapRemoteShift", () => {
  it("maps a leave payload as a pending remote request", () => {
    const leave = mapRemoteLeave(operation("leave", { id: "l1", leave_type: "sick", start_date: "2026-08-21", end_date: "2026-08-22", reason: "flu" }));
    expect(leave).toEqual({ id: "l1", requested_by: "remote", leave_type: "sick", start_date: "2026-08-21", end_date: "2026-08-22", reason: "flu", status: "pending" });
  });

  it("defaults the leave type to annual", () => {
    expect(mapRemoteLeave(operation("leave", { id: "l2", start_date: "2026-08-21", end_date: "2026-08-22" })).leave_type).toBe("annual");
  });

  it("maps a shift payload as scheduled", () => {
    const shift = mapRemoteShift(operation("shift", { id: "s1", assigned_to: "u2", title: "Front desk", shift_date: "2026-08-25", starts_at: "09:00", ends_at: "17:00", note: "" }));
    expect(shift).toMatchObject({ id: "s1", assigned_to: "u2", status: "scheduled", title: "Front desk" });
  });
});

describe("hasId", () => {
  it("is false when the payload has no id", () => {
    expect(hasId(operation("task", { title: "no id" }))).toBe(false);
    expect(hasId(operation("task", { id: "" }))).toBe(false);
    expect(hasId(operation("task", { id: "t1" }))).toBe(true);
  });
});
