import { describe, expect, it } from "vitest";
import {
  applyPullOperations,
  applyPushResponse,
  countPushOutcomes,
  emptyCache,
  isOfflineCapable,
  isServerAuthoritative,
  queueOfflineMutation,
} from "./offline-harness";
import type { RemoteOperation } from "./sync-transform";

const op = (entity: string, action: string, payload: Record<string, unknown>, id = "op-1"): RemoteOperation => ({
  id,
  entity,
  action,
  payload,
  created_at: "2026-08-20T12:00:00Z",
});

describe("offline capability policy", () => {
  it("allows device-offline mutations listed in OFFLINE_RULES", () => {
    expect(isOfflineCapable("task", "create")).toBe(true);
    expect(isOfflineCapable("task", "delete")).toBe(true);
    expect(isOfflineCapable("attendance", "check_in")).toBe(true);
    expect(isOfflineCapable("leave", "create")).toBe(true);
    expect(isOfflineCapable("shift", "create")).toBe(true);
  });

  it("blocks server-authoritative actions from the outbox", () => {
    expect(isServerAuthoritative("leave", "approve")).toBe(true);
    expect(isServerAuthoritative("workflow", "reject")).toBe(true);
    expect(isServerAuthoritative("finance", "pay")).toBe(true);
    expect(isServerAuthoritative("rbac", "assign")).toBe(true);
    expect(isServerAuthoritative("auth", "change_password")).toBe(true);
    expect(isOfflineCapable("leave", "approve")).toBe(false);
  });
});

describe("outbox push status transitions", () => {
  it("marks accepted and duplicate ids as synced (idempotent replay)", () => {
    const statuses = applyPushResponse(["a", "b", "c", "d"], {
      accepted: ["a"],
      duplicates: ["b"],
      rejected: ["c"],
      conflicts: ["d"],
    });
    expect(statuses.get("a")).toBe("synced");
    expect(statuses.get("b")).toBe("synced");
    expect(statuses.get("c")).toBe("failed");
    expect(statuses.get("d")).toBe("conflict");
    expect(countPushOutcomes(statuses)).toEqual({ synced: 2, failed: 1, conflicts: 1, pending: 0 });
  });

  it("keeps unknown pending ids pending when the server omits them", () => {
    const statuses = applyPushResponse(["x", "y"], { accepted: ["x"] });
    expect(statuses.get("y")).toBe("pending");
  });
});

describe("queueOfflineMutation", () => {
  it("queues a valid offline create and rejects duplicates and forbidden actions", () => {
    const first = queueOfflineMutation([], {
      id: "task:1",
      entity: "task",
      action: "create",
      payload: { id: "t1", title: "Ship" },
      createdAt: "2026-08-20T12:00:00Z",
    });
    expect(first.ok).toBe(true);
    expect(first.outbox).toHaveLength(1);

    const dup = queueOfflineMutation(first.outbox, first.outbox[0]);
    expect(dup.ok).toBe(false);
    expect(dup.reason).toBe("duplicate operation id");

    const forbidden = queueOfflineMutation([], {
      id: "leave:approve:1",
      entity: "leave",
      action: "approve",
      payload: { id: "l1" },
      createdAt: "2026-08-20T12:00:00Z",
    });
    expect(forbidden.ok).toBe(false);
    expect(forbidden.reason).toBe("server-authoritative");
  });
});

describe("pull feed application (device reconnect)", () => {
  it("upserts tasks and drops them when a tombstone arrives", () => {
    let cache = emptyCache();
    cache = applyPullOperations(cache, [
      op("task", "create", { id: "t1", title: "One", status: "todo", priority: "normal" }),
      op("task", "create", { id: "t2", title: "Two" }, "op-2"),
    ]);
    expect(cache.tasks.size).toBe(2);
    expect(cache.tasks.get("t1")?.title).toBe("One");

    cache = applyPullOperations(cache, [op("task", "delete", { id: "t1" }, "op-del")]);
    expect(cache.tasks.has("t1")).toBe(false);
    expect(cache.tasks.has("t2")).toBe(true);
  });

  it("merges attendance check-in and check-out on the same work date", () => {
    let cache = emptyCache();
    cache = applyPullOperations(cache, [
      op("attendance", "check_in", { id: "a1", work_date: "2026-08-20", at: "2026-08-20T09:00:00Z" }, "in"),
    ]);
    cache = applyPullOperations(cache, [
      op("attendance", "check_out", { id: "a1", work_date: "2026-08-20", at: "2026-08-20T17:00:00Z" }, "out"),
    ]);
    const day = cache.attendanceByDate.get("2026-08-20");
    expect(day?.check_in_at).toBe("2026-08-20T09:00:00Z");
    expect(day?.check_out_at).toBe("2026-08-20T17:00:00Z");
    expect(cache.attendanceByDate.size).toBe(1);
  });

  it("caches leave and shift creates from other clients", () => {
    let cache = emptyCache();
    cache = applyPullOperations(cache, [
      op("leave", "create", { id: "l1", leave_type: "annual", start_date: "2026-08-21", end_date: "2026-08-22" }, "leave-1"),
      op("shift", "create", { id: "s1", title: "Desk", shift_date: "2026-08-21", starts_at: "09:00", ends_at: "17:00" }, "shift-1"),
    ]);
    expect(cache.leave.get("l1")?.status).toBe("pending");
    expect(cache.shifts.get("s1")?.status).toBe("scheduled");
  });
});

describe("device-offline reconnect scenario", () => {
  it("queues while offline, then applies push outcomes and a pull tombstone", () => {
    const queued = queueOfflineMutation([], {
      id: "task:offline-1",
      entity: "task",
      action: "create",
      payload: { id: "t-offline", title: "Offline note" },
      createdAt: "2026-08-20T10:00:00Z",
    });
    expect(queued.ok).toBe(true);

    const push = applyPushResponse(
      queued.outbox.map((item) => item.id),
      { accepted: ["task:offline-1"] },
    );
    expect(countPushOutcomes(push)).toMatchObject({ synced: 1, failed: 0, conflicts: 0 });

    let cache = emptyCache();
    cache = applyPullOperations(cache, [
      op("task", "create", { id: "t-offline", title: "Offline note" }, "task:offline-1"),
      op("task", "create", { id: "t-other", title: "From peer" }, "peer-1"),
    ]);
    expect(cache.tasks.size).toBe(2);

    cache = applyPullOperations(cache, [op("task", "delete", { id: "t-other" }, "peer-del")]);
    expect(cache.tasks.has("t-other")).toBe(false);
    expect(cache.tasks.has("t-offline")).toBe(true);
  });

  it("surfaces a create conflict without marking it synced", () => {
    const statuses = applyPushResponse(["task:dup"], {
      conflicts: ["task:dup"],
    });
    expect(statuses.get("task:dup")).toBe("conflict");
    expect(countPushOutcomes(statuses).synced).toBe(0);
  });
});
