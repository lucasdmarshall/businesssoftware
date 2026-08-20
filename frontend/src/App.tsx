import { useEffect, useState, type FormEvent } from "react";
import {
  Activity,
  ArrowUpRight,
  Bell,
  BriefcaseBusiness,
  ChevronDown,
  CircleHelp,
  ClipboardCheck,
  Clock3,
  CalendarDays,
  FolderKanban,
  LayoutDashboard,
  Moon,
  PanelLeft,
  Search,
  Sun,
  UsersRound,
} from "lucide-react";
import { cacheAttendance, cacheLeave, cacheSession, cacheShifts, cacheTasks, discardOperation, getCachedSession, getLocalAttendance, getLocalLeave, getLocalShifts, getLocalTasks, getOutboxStatuses, initializeLocalDb, pullRemoteChanges, queueOperation, retryOperation, syncPendingOperations, type LocalAttendance, type LocalLeave, type LocalShift, type LocalTask } from "./localDb";

type Theme = "light" | "dark";
type SystemHealth = { status: string; database: string };
type Organization = { id: string; name: string; slug: string };
type AuthState = "loading" | "setup" | "login" | "authenticated";
type CurrentUser = { name: string; email: string; organization: string; role: string; permissions?: string[] };
type UserRecord = { id: string; email: string; display_name: string; status: string };
type DepartmentRecord = { id: string; name: string; slug: string };
type LeaveRequest = { id: string; requested_by: string; display_name?: string; leave_type: string; start_date: string; end_date: string; reason: string; status: string };
type RoleRecord = { id: string; code: string; name: string; permissions: string[] };
type PermissionRecord = { code: string; description: string };
type AuditEntry = { id: string; action: string; entity_type: string; entity_id: string; actor_name: string; created_at: string };
type WorkflowStepInput = { name: string; approver_role_code: string; required_approvals: number; min_amount: number | null; max_amount: number | null };
type WorkflowStep = WorkflowStepInput & { id: string; step_order: number; approver_user_id: string };
type WorkflowDefinition = { id: string; code: string; name: string; entity_type: string; active: boolean; steps: WorkflowStep[] };
type WorkflowActionEntry = { id: string; step_order: number | null; actor_name: string; action: string; reason: string; created_at: string };
type WorkflowInstance = { id: string; definition_id: string; definition_name: string; title: string; entity_type: string; amount: number | null; status: string; current_step_order: number | null; current_step_name: string; submitted_by: string; submitter_name: string; created_at: string; updated_at: string; actions?: WorkflowActionEntry[] };

type NotificationItem = { id: string; kind: string; title: string; body: string; entity_type: string; entity_id: string; read_at: string | null; created_at: string };
type ProjectItem = { id: string; key: string; name: string; description: string; status: string; lead_id: string; lead_name: string; task_count: number; created_at: string };
type CalendarEvent = { id: string; title: string; description: string; starts_at: string; ends_at: string; all_day: boolean; visibility: string; creator_name: string };

const workflowStatusClass: Record<string, string> = { in_review: "leave-pending", approved: "leave-approved", rejected: "leave-rejected", cancelled: "leave-rejected", draft: "" };

const viewForLabel = (label: string): string => label === "People" ? "people" : label === "Work" ? "work" : label === "Projects" ? "projects" : label === "Approvals" ? "approvals" : label === "Calendar" ? "calendar" : label === "Attendance" ? "attendance" : label === "Leave" ? "leave" : label === "Schedule" ? "schedule" : label === "Activity" ? "activity" : "overview";

const apiBase = "http://localhost:8080/api/v1";

const navigation = [
  { label: "Overview", icon: LayoutDashboard, active: true },
  { label: "Work", icon: BriefcaseBusiness, permission: "tasks.read" },
  { label: "Projects", icon: FolderKanban, permission: "projects.read" },
  { label: "Approvals", icon: ClipboardCheck, permission: "workflow.read" },
  { label: "Calendar", icon: CalendarDays, permission: "calendar.read" },
  { label: "Attendance", icon: Clock3, permission: "attendance.read" },
  { label: "Leave", icon: CalendarDays, permission: "leave.read" },
  { label: "Schedule", icon: CalendarDays, permission: "shifts.read" },
  { label: "People", icon: UsersRound, permission: "users.read" },
  { label: "Activity", icon: Activity, permission: "organization.read" },
];

function App() {
  const [theme, setTheme] = useState<Theme>(() => {
    return (localStorage.getItem("name-theme") as Theme) || "dark";
  });
  const [systemHealth, setSystemHealth] = useState<SystemHealth>({ status: "offline", database: "unknown" });
  const [organizations, setOrganizations] = useState<Organization[]>([]);
  const [authState, setAuthState] = useState<AuthState>("loading");
  const [currentUser, setCurrentUser] = useState<CurrentUser | null>(null);
  const [activeView, setActiveView] = useState("overview");

  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    localStorage.setItem("name-theme", theme);
  }, [theme]);

  useEffect(() => {
    const loadWorkspace = async () => {
      try {
        await initializeLocalDb();
      } catch {
        // Browser development mode can run without the native Tauri SQLite plugin.
      }
      try {
        const healthResponse = await fetch(`${apiBase}/health`);
        if (!healthResponse.ok) throw new Error("Backend unavailable");
        setSystemHealth(await healthResponse.json());

        const setupResponse = await fetch(`${apiBase}/setup/status`);
        if (setupResponse.ok && (await setupResponse.json()).needs_setup) {
          setAuthState("setup");
          return;
        }

        const meResponse = await fetch(`${apiBase}/auth/me`, { credentials: "include" });
        if (meResponse.ok) {
          setCurrentUser(await meResponse.json());
          setAuthState("authenticated");
          // Bring the local read caches up to date with other clients' synced
          // changes so this device keeps working if it later goes offline.
          try {
            await syncPendingOperations(apiBase);
            await pullRemoteChanges(apiBase);
          } catch {
            // Running outside Tauri (no local SQLite) or a transient error.
          }
        } else {
          setAuthState("login");
        }

        const organizationsResponse = await fetch(`${apiBase}/organizations`);
        if (organizationsResponse.ok) setOrganizations(await organizationsResponse.json());
      } catch {
        setSystemHealth({ status: "offline", database: "unavailable" });
        try {
          const cached = await getCachedSession();
          if (cached) {
            setCurrentUser({ name: cached.displayName, email: cached.email, organization: cached.organization, role: cached.role, permissions: cached.permissions });
            setAuthState("authenticated");
            return;
          }
        } catch {
          // No local database is available yet.
        }
        setAuthState("login");
      }
    };

    void loadWorkspace();
  }, []);

  const toggleTheme = () => setTheme((current) => (current === "dark" ? "light" : "dark"));
  const handleAuthenticated = async (user: CurrentUser) => {
    setCurrentUser(user);
    setAuthState("authenticated");
    try {
      await cacheSession({ email: user.email, displayName: user.name, organization: user.organization, role: user.role, permissions: user.permissions ?? [], cachedAt: new Date().toISOString() });
      await syncPendingOperations(apiBase);
      await pullRemoteChanges(apiBase);
    } catch {
      // Keep online auth working when running the frontend outside Tauri or offline.
    }
  };

  if (authState === "loading") return <LoadingScreen />;
  if (authState === "setup" || authState === "login") {
    return <AuthScreen mode={authState} theme={theme} onThemeToggle={toggleTheme} onAuthenticated={handleAuthenticated} />;
  }

  return (
    <div className="app-shell">
      <a className="skip-link" href="#main-content">Skip to content</a>
      <aside className="sidebar">
        <div className="brand-mark">N</div>
        <div className="workspace-switcher">
          <span className="workspace-avatar">A</span>
          <span className="workspace-name">Acme workspace</span>
          <ChevronDown size={15} />
        </div>

        <nav className="primary-nav" aria-label="Primary navigation">
          <p className="eyebrow">Workspace</p>
          {navigation.filter(({ permission }) => !permission || !currentUser?.permissions?.length || currentUser.permissions.includes(permission)).map(({ label, icon: Icon }) => {
            const view = viewForLabel(label);
            const isActive = activeView === view;
            return (
              <button className={`nav-item ${isActive ? "active" : ""}`} aria-current={isActive ? "page" : undefined} key={label} type="button" onClick={() => setActiveView(view)}>
                <Icon size={17} strokeWidth={1.8} aria-hidden="true" />
                <span>{label}</span>
              </button>
            );
          })}
        </nav>

        <div className="sidebar-footer">
          <button className="nav-item" type="button">
            <CircleHelp size={17} strokeWidth={1.8} />
            <span>Help center</span>
          </button>
          <div className="profile-row">
            <span className="profile-avatar">L</span>
            <span className="profile-copy"><strong>Lucas</strong><small>Administrator</small></span>
            <PanelLeft size={15} />
          </div>
        </div>
      </aside>

      <main className="main-content" id="main-content">
        <header className="topbar">
          <div className="breadcrumb">Overview <span>/</span> Workspace</div>
          <div className="topbar-actions">
            <button className="search-button" type="button"><Search size={16} aria-hidden="true" /> Search <kbd>⌘ K</kbd></button>
            <NotificationBell />
            <span className={`status-dot ${systemHealth.status !== "ok" ? "offline" : ""}`} role="status" aria-label={`Backend ${systemHealth.status}, database ${systemHealth.database}`} title={`Backend: ${systemHealth.status} · Database: ${systemHealth.database}`} />
          </div>
        </header>

        {activeView === "people" ? <PeopleView /> : activeView === "work" ? <WorkView /> : activeView === "projects" ? <ProjectsView /> : activeView === "approvals" ? <WorkflowView /> : activeView === "calendar" ? <CalendarView /> : activeView === "attendance" ? <AttendanceView /> : activeView === "leave" ? <LeaveView /> : activeView === "schedule" ? <ScheduleView /> : activeView === "activity" ? <ActivityView /> : <section className="content-wrap">
          <div className="page-heading">
            <div>
              <p className="eyebrow">Wednesday, 20 August 2026</p>
              <h1>Good morning, Lucas.</h1>
              <p className="lede">Here is what is happening across your workspace.</p>
            </div>
            <button className="primary-button" type="button">Open workspace <ArrowUpRight size={16} /></button>
          </div>

          <div className="connection-strip">
            <span className={`connection-icon ${systemHealth.status === "ok" ? "ready" : "offline"}`}><Activity size={15} /></span>
            <span><strong>{organizations[0]?.name ?? currentUser?.organization ?? "Company setup"}</strong><small>{systemHealth.status === "ok" ? `Backend connected · PostgreSQL ${systemHealth.database}` : "Start the local backend to connect this workspace"}</small></span>
          </div>

          <DashboardOverview />
        </section>}
      </main>

      <button className="theme-switcher" type="button" onClick={toggleTheme} aria-label={`Switch to ${theme === "dark" ? "light" : "dark"} theme`}>
        {theme === "dark" ? <Sun size={17} /> : <Moon size={17} />}
        <span>{theme === "dark" ? "Light" : "Dark"}</span>
      </button>
    </div>
  );
}

function WorkView() {
  const [tasks, setTasks] = useState<LocalTask[]>([]);
  const [outboxStatuses, setOutboxStatuses] = useState<Map<string, string>>(new Map());
  const [title, setTitle] = useState("");
  const [priority, setPriority] = useState("normal");
  const [dueAt, setDueAt] = useState("");
  const [assignedTo, setAssignedTo] = useState("");
  const [recurrenceRule, setRecurrenceRule] = useState("");
  const [taskScope, setTaskScope] = useState("organization");
  const [users, setUsers] = useState<UserRecord[]>([]);
  const [message, setMessage] = useState("");
  const [saving, setSaving] = useState(false);
  const [commentTaskId, setCommentTaskId] = useState<string | null>(null);
  const [comments, setComments] = useState<Array<{ id: string; author_name: string; body: string }>>([]);
  const [commentBody, setCommentBody] = useState("");

  useEffect(() => {
    const loadTasks = async () => {
      try {
        await syncPendingOperations(apiBase);
        const response = await fetch(`${apiBase}/tasks?scope=${taskScope}`, { credentials: "include" });
        if (!response.ok) throw new Error("offline");
        const remoteTasks = await response.json() as Array<Record<string, string>>;
        const normalized = remoteTasks.map((task) => ({ id: task.id, title: task.title, description: task.description ?? "", status: task.status, priority: task.priority, dueAt: task.due_at ?? null, assignedTo: task.assigned_to ?? null, recurrenceRule: task.recurrence_rule ?? null, recurrenceNextAt: task.recurrence_next_at ?? null, createdAt: task.created_at, updatedAt: task.updated_at }));
        setTasks(normalized);
        await cacheTasks(normalized);
        const usersResponse = await fetch(`${apiBase}/users`, { credentials: "include" }); if (usersResponse.ok) setUsers(await usersResponse.json());
      } catch {
        try { setTasks(await getLocalTasks()); } catch { setTasks([]); }
      }
    };
    void loadTasks();
    const handleOnline = () => { void loadTasks(); };
    window.addEventListener("online", handleOnline);
    return () => window.removeEventListener("online", handleOnline);
  }, [taskScope]);

  useEffect(() => {
    void getOutboxStatuses(tasks.map((task) => task.id)).then(setOutboxStatuses).catch(() => undefined);
  }, [tasks]);

  const retryTask = async (taskId: string) => {
    await retryOperation(taskId);
    setOutboxStatuses((current) => new Map(current).set(taskId, "pending"));
    try { await syncPendingOperations(apiBase); } catch { /* remain pending while offline */ }
    const refreshed = await getLocalTasks();
    setTasks(refreshed);
  };

  const discardTask = async (taskId: string) => {
    await discardOperation(taskId);
    setOutboxStatuses((current) => { const next = new Map(current); next.delete(taskId); return next; });
  };

  const openComments = async (taskId: string) => { setCommentTaskId(taskId); const response = await fetch(`${apiBase}/tasks/${taskId}/comments`, { credentials: "include" }); if (response.ok) setComments(await response.json()); };
  const addComment = async (event: FormEvent<HTMLFormElement>) => { event.preventDefault(); if (!commentTaskId || !commentBody.trim()) return; const response = await fetch(`${apiBase}/tasks/${commentTaskId}/comments`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ body: commentBody }) }); if (response.ok) { const created = await response.json(); setComments((current) => [...current, created]); setCommentBody(""); } };

  const createTask = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!title.trim()) return;
    setSaving(true);
    setMessage("");
    const now = new Date().toISOString();
    const recurrenceNextAt = recurrenceRule && dueAt ? new Date(`${dueAt}T23:59:00`).toISOString() : null;
    const localTask: LocalTask = { id: crypto.randomUUID(), title: title.trim(), description: "", status: "todo", priority, dueAt: dueAt ? new Date(`${dueAt}T23:59:00`).toISOString() : null, assignedTo: assignedTo || null, recurrenceRule: recurrenceRule || null, recurrenceNextAt, createdAt: now, updatedAt: now };
    try {
      const response = await fetch(`${apiBase}/tasks`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ title: localTask.title, priority: localTask.priority, due_at: localTask.dueAt, assigned_to: localTask.assignedTo, recurrence_rule: localTask.recurrenceRule, recurrence_next_at: localTask.recurrenceNextAt }) });
      if (!response.ok) throw new Error("offline");
      const remote = await response.json() as Record<string, string>;
      const saved: LocalTask = { id: remote.id, title: remote.title, description: remote.description ?? "", status: remote.status, priority: remote.priority, dueAt: remote.due_at ?? null, assignedTo: remote.assigned_to ?? null, recurrenceRule: remote.recurrence_rule ?? null, recurrenceNextAt: remote.recurrence_next_at ?? null, createdAt: remote.created_at, updatedAt: remote.updated_at };
      setTasks((current) => [saved, ...current]);
      await cacheTasks([saved]);
      setMessage("Task created");
    } catch {
      await queueOperation({ id: localTask.id, entity: "task", action: "create", payload: { id: localTask.id, title: localTask.title, priority: localTask.priority, status: localTask.status, description: "", due_at: localTask.dueAt, assigned_to: localTask.assignedTo, recurrence_rule: localTask.recurrenceRule, recurrence_next_at: localTask.recurrenceNextAt }, createdAt: now });
      await cacheTasks([localTask]);
      setTasks((current) => [localTask, ...current]);
      setMessage("Saved offline · will sync when the server is reachable");
    } finally {
      setTitle("");
      setDueAt(""); setAssignedTo(""); setRecurrenceRule("");
      setSaving(false);
    }
  };

  return <section className="content-wrap">
    <div className="page-heading"><div><p className="eyebrow">Work</p><h1>Keep work moving.</h1><p className="lede">Tasks stay available on this device, even when the company server is offline.</p></div></div>
    {message && <p className="inline-message">{message}</p>}
    <form className="task-composer" onSubmit={createTask}><input value={title} onChange={(event) => setTitle(event.target.value)} placeholder="What needs to get done?" required /><select value={priority} onChange={(event) => setPriority(event.target.value)}><option value="low">Low priority</option><option value="normal">Normal priority</option><option value="high">High priority</option><option value="urgent">Urgent</option></select><input type="date" value={dueAt} onChange={(event) => setDueAt(event.target.value)} /><select value={assignedTo} onChange={(event) => setAssignedTo(event.target.value)}><option value="">Assign to me</option>{users.map((user) => <option key={user.id} value={user.id}>{user.display_name}</option>)}</select><select value={recurrenceRule} onChange={(event) => setRecurrenceRule(event.target.value)}><option value="">No repeat</option><option value="daily">Daily</option><option value="weekly">Weekly</option><option value="monthly">Monthly</option></select><button className="primary-button" type="submit" disabled={saving}>{saving ? "Saving…" : "Add task"}</button></form>
    <div className="section-heading task-heading"><div><p className="eyebrow">Queue</p><h2>{tasks.length} tasks</h2></div><select className="scope-select" value={taskScope} onChange={(event) => setTaskScope(event.target.value)}><option value="own">My tasks</option><option value="team">Team tasks</option><option value="department">Department tasks</option><option value="organization">All company tasks</option></select></div>
    <div className="task-list">{tasks.map((task) => { const syncStatus = outboxStatuses.get(task.id); return <div key={task.id}><div className="task-row"><span className={`task-status ${task.status}`} /><span className="record-copy"><strong>{task.title}</strong><small>{task.priority} priority · {task.dueAt ? `due ${task.dueAt.slice(0, 10)} · ` : ""}{task.assignedTo ? "assigned" : "unassigned"} · {syncStatus === "conflict" ? "server conflict" : syncStatus === "failed" ? "sync failed" : syncStatus === "pending" ? "waiting to sync" : "synced"}</small></span><button className="text-button" type="button" onClick={() => void openComments(task.id)}>Comments</button>{syncStatus === "failed" || syncStatus === "conflict" ? <span className="task-actions"><button className="text-button" type="button" onClick={() => void retryTask(task.id)}>Retry</button>{syncStatus === "conflict" && <button className="text-button muted" type="button" onClick={() => void discardTask(task.id)}>Discard</button>}</span> : <span className={`status-pill priority-${task.priority}`}>{task.status.replace("_", " ")}</span>}</div>{commentTaskId === task.id && <div className="comment-thread">{comments.map((comment) => <p key={comment.id}><strong>{comment.author_name}</strong> {comment.body}</p>)}<form onSubmit={addComment}><input value={commentBody} onChange={(event) => setCommentBody(event.target.value)} placeholder="Write a comment or @mention someone" /><button className="text-button" type="submit">Post</button></form></div>}</div>; })}</div>
  </section>;
}

function AttendanceView() {
  const [records, setRecords] = useState<LocalAttendance[]>([]);
  const [managerRecords, setManagerRecords] = useState<Array<LocalAttendance & { display_name?: string }>>([]);
  const [users, setUsers] = useState<Array<{ id: string; display_name: string }>>([]);
  const [correction, setCorrection] = useState({ user_id: "", work_date: new Date().toISOString().slice(0, 10), check_in_at: "", check_out_at: "", status: "present", note: "" });
  const [message, setMessage] = useState("");
  const today = new Date().toISOString().slice(0, 10);
  const todayRecord = records.find((record) => record.work_date === today);

  const load = async () => {
    try {
      await syncPendingOperations(apiBase);
      const response = await fetch(`${apiBase}/attendance`, { credentials: "include" });
      if (!response.ok) throw new Error("offline");
      const remote = await response.json() as LocalAttendance[];
      setRecords(remote); await cacheAttendance(remote);
      const managerResponse = await fetch(`${apiBase}/attendance/organization`, { credentials: "include" });
      if (managerResponse.ok) {
        setManagerRecords(await managerResponse.json());
        const usersResponse = await fetch(`${apiBase}/users`, { credentials: "include" });
        if (usersResponse.ok) setUsers(await usersResponse.json());
      }
    } catch { try { setRecords(await getLocalAttendance()); } catch { setRecords([]); } }
  };
  useEffect(() => { void load(); const onOnline = () => void load(); window.addEventListener("online", onOnline); return () => window.removeEventListener("online", onOnline); }, []);

  const mark = async (kind: "check_in" | "check_out") => {
    const now = new Date().toISOString();
    const local: LocalAttendance = todayRecord ?? { id: crypto.randomUUID(), user_id: "local", work_date: today, check_in_at: null, check_out_at: null, status: "present", note: "" };
    if (kind === "check_in") local.check_in_at = now; else local.check_out_at = now;
    try {
      const response = await fetch(`${apiBase}/attendance/${kind.replace("_", "-")}`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ id: local.id, work_date: today, at: now }) });
      if (!response.ok) throw new Error("offline");
      const saved = await response.json() as LocalAttendance; setRecords((current) => [saved, ...current.filter((item) => item.work_date !== today)]); await cacheAttendance([saved]); setMessage(kind === "check_in" ? "Checked in" : "Checked out");
    } catch {
      await queueOperation({ id: `attendance:${today}:${kind}`, entity: "attendance", action: kind, payload: { id: local.id, work_date: today, at: now, note: "" }, createdAt: now });
      await cacheAttendance([local]); setRecords((current) => [local, ...current.filter((item) => item.work_date !== today)]); setMessage("Saved offline · will sync when the server is reachable");
    }
  };

  const submitCorrection = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const response = await fetch(`${apiBase}/attendance/corrections`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ ...correction, check_in_at: correction.check_in_at ? new Date(`${correction.work_date}T${correction.check_in_at}`).toISOString() : null, check_out_at: correction.check_out_at ? new Date(`${correction.work_date}T${correction.check_out_at}`).toISOString() : null }) });
    if (!response.ok) { setMessage("Correction could not be saved"); return; }
    setMessage("Attendance corrected"); setCorrection((current) => ({ ...current, check_in_at: "", check_out_at: "", note: "" })); void load();
  };

  return <section className="content-wrap"><div className="page-heading"><div><p className="eyebrow">Attendance</p><h1>Be present, clearly.</h1><p className="lede">Check-in and check-out remain available when the company server is offline.</p></div></div>{message && <p className="inline-message">{message}</p>}<div className="attendance-today"><div><p className="eyebrow">Today · {today}</p><h2>{todayRecord?.check_in_at ? todayRecord.check_out_at ? "Workday complete" : "Currently working" : "Not checked in"}</h2></div><div className="attendance-actions"><button className="primary-button" type="button" onClick={() => void mark("check_in")} disabled={Boolean(todayRecord?.check_in_at)}>Check in</button><button className="text-button" type="button" onClick={() => void mark("check_out")} disabled={!todayRecord?.check_in_at || Boolean(todayRecord.check_out_at)}>Check out</button></div></div><div className="section-heading task-heading"><div><p className="eyebrow">History</p><h2>Recent attendance</h2></div></div><div className="record-list">{records.map((record) => <div className="record-row" key={record.id}><span className="record-avatar"><Clock3 size={15} /></span><span className="record-copy"><strong>{record.work_date}</strong><small>{record.check_in_at ? new Date(record.check_in_at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }) : "—"} → {record.check_out_at ? new Date(record.check_out_at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }) : "—"}</small></span><span className="status-pill">{record.status}</span></div>)}</div>{managerRecords.length > 0 && <><div className="section-heading task-heading"><div><p className="eyebrow">Manager view</p><h2>Team attendance</h2></div></div><div className="record-list">{managerRecords.map((record) => <div className="record-row" key={record.id}><span className="record-avatar"><UsersRound size={15} /></span><span className="record-copy"><strong>{record.display_name}</strong><small>{record.work_date} · {record.check_in_at ? new Date(record.check_in_at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }) : "—"} → {record.check_out_at ? new Date(record.check_out_at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }) : "—"}</small></span><span className="status-pill">{record.status}</span></div>)}</div><form className="compact-form attendance-correction" onSubmit={submitCorrection}><p className="eyebrow">Correction</p><select value={correction.user_id} onChange={(event) => setCorrection({ ...correction, user_id: event.target.value })} required><option value="">Select person</option>{users.map((user) => <option value={user.id} key={user.id}>{user.display_name}</option>)}</select><input type="date" value={correction.work_date} onChange={(event) => setCorrection({ ...correction, work_date: event.target.value })} required /><div className="form-grid"><input type="time" value={correction.check_in_at} onChange={(event) => setCorrection({ ...correction, check_in_at: event.target.value })} /><input type="time" value={correction.check_out_at} onChange={(event) => setCorrection({ ...correction, check_out_at: event.target.value })} /><input placeholder="Reason or note" value={correction.note} onChange={(event) => setCorrection({ ...correction, note: event.target.value })} /></div><button className="primary-button" type="submit">Save correction</button></form></>}</section>;
}

function LeaveView() {
  const [requests, setRequests] = useState<Array<LeaveRequest & LocalLeave>>([]);
  const [form, setForm] = useState({ leave_type: "annual", start_date: new Date().toISOString().slice(0, 10), end_date: new Date().toISOString().slice(0, 10), reason: "" });
  const [message, setMessage] = useState("");
  const load = async () => { try { await syncPendingOperations(apiBase); const response = await fetch(`${apiBase}/leave`, { credentials: "include" }); if (!response.ok) throw new Error("offline"); const remote = await response.json() as Array<LeaveRequest & LocalLeave>; setRequests(remote); await cacheLeave(remote); } catch { try { setRequests(await getLocalLeave() as Array<LeaveRequest & LocalLeave>); } catch { setRequests([]); } } };
  useEffect(() => { void load(); const onOnline = () => void load(); window.addEventListener("online", onOnline); return () => window.removeEventListener("online", onOnline); }, []);
  const createRequest = async (event: FormEvent<HTMLFormElement>) => { event.preventDefault(); const now = new Date().toISOString(); const local: LocalLeave = { id: crypto.randomUUID(), requested_by: "local", leave_type: form.leave_type, start_date: form.start_date, end_date: form.end_date, reason: form.reason, status: "pending" }; try { const response = await fetch(`${apiBase}/leave`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify(form) }); if (!response.ok) throw new Error("offline"); const saved = await response.json() as LeaveRequest & LocalLeave; setRequests((current) => [saved, ...current]); await cacheLeave([saved]); setMessage("Leave request submitted"); } catch { await queueOperation({ id: `leave:${local.id}`, entity: "leave", action: "create", payload: local, createdAt: now }); await cacheLeave([local]); setRequests((current) => [local, ...current]); setMessage("Saved offline · will sync when the server is reachable"); } setForm((current) => ({ ...current, reason: "" })); };
  const decide = async (id: string, action: "approve" | "reject") => { const response = await fetch(`${apiBase}/leave/${action}`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ id }) }); setMessage(response.ok ? `Request ${action}d` : "You do not have permission to decide this request"); if (response.ok) void load(); };
  return <section className="content-wrap"><div className="page-heading"><div><p className="eyebrow">Leave</p><h1>Plan time away.</h1><p className="lede">Requests stay visible to the team and move through a clear approval path.</p></div></div>{message && <p className="inline-message">{message}</p>}<form className="leave-form" onSubmit={createRequest}><p className="eyebrow">New request</p><div className="form-grid"><select value={form.leave_type} onChange={(event) => setForm({ ...form, leave_type: event.target.value })}><option value="annual">Annual leave</option><option value="sick">Sick leave</option><option value="personal">Personal leave</option><option value="unpaid">Unpaid leave</option></select><input type="date" value={form.start_date} onChange={(event) => setForm({ ...form, start_date: event.target.value })} required /><input type="date" value={form.end_date} onChange={(event) => setForm({ ...form, end_date: event.target.value })} required /></div><input placeholder="Reason (optional)" value={form.reason} onChange={(event) => setForm({ ...form, reason: event.target.value })} /><button className="primary-button" type="submit">Submit request</button></form><div className="section-heading task-heading"><div><p className="eyebrow">Requests</p><h2>{requests.length} requests</h2></div></div><div className="record-list">{requests.map((request) => <div className="record-row" key={request.id}><span className="record-avatar"><CalendarDays size={15} /></span><span className="record-copy"><strong>{request.display_name ?? "My request"} · {request.leave_type}</strong><small>{request.start_date} → {request.end_date}{request.reason ? ` · ${request.reason}` : ""}</small></span><span className={`status-pill leave-${request.status}`}>{request.status}</span>{request.status === "pending" && <span className="task-actions"><button className="text-button" type="button" onClick={() => void decide(request.id, "approve")}>Approve</button><button className="text-button muted" type="button" onClick={() => void decide(request.id, "reject")}>Reject</button></span>}</div>)}</div></section>;
}

function ScheduleView() {
  const [shifts, setShifts] = useState<LocalShift[]>([]);
  const [form, setForm] = useState({ title: "", shift_date: new Date().toISOString().slice(0, 10), starts_at: "09:00", ends_at: "17:00", note: "" });
  const [message, setMessage] = useState("");
  const load = async () => { try { await syncPendingOperations(apiBase); const response = await fetch(`${apiBase}/shifts`, { credentials: "include" }); if (!response.ok) throw new Error("offline"); const remote = await response.json() as LocalShift[]; setShifts(remote); await cacheShifts(remote); } catch { try { setShifts(await getLocalShifts()); } catch { setShifts([]); } } };
  useEffect(() => { void load(); const onOnline = () => void load(); window.addEventListener("online", onOnline); return () => window.removeEventListener("online", onOnline); }, []);
  const createShift = async (event: FormEvent<HTMLFormElement>) => { event.preventDefault(); const now = new Date().toISOString(); const local: LocalShift = { id: crypto.randomUUID(), assigned_to: "local", ...form, status: "scheduled" }; try { const response = await fetch(`${apiBase}/shifts`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify(form) }); if (!response.ok) throw new Error("offline"); const saved = await response.json() as LocalShift; setShifts((current) => [...current, saved]); await cacheShifts([saved]); setMessage("Shift scheduled"); } catch { await queueOperation({ id: `shift:${local.id}`, entity: "shift", action: "create", payload: local, createdAt: now }); await cacheShifts([local]); setShifts((current) => [...current, local]); setMessage("Saved offline · will sync when the server is reachable"); } setForm((current) => ({ ...current, title: "", note: "" })); };
  return <section className="content-wrap"><div className="page-heading"><div><p className="eyebrow">Schedule</p><h1>Make time visible.</h1><p className="lede">Shifts stay available on this device and sync back to the company schedule.</p></div></div>{message && <p className="inline-message">{message}</p>}<form className="leave-form" onSubmit={createShift}><p className="eyebrow">New shift</p><div className="form-grid"><input placeholder="Shift title" value={form.title} onChange={(event) => setForm({ ...form, title: event.target.value })} required /><input type="date" value={form.shift_date} onChange={(event) => setForm({ ...form, shift_date: event.target.value })} required /><input type="time" value={form.starts_at} onChange={(event) => setForm({ ...form, starts_at: event.target.value })} required /></div><div className="form-grid"><input type="time" value={form.ends_at} onChange={(event) => setForm({ ...form, ends_at: event.target.value })} required /><input placeholder="Note (optional)" value={form.note} onChange={(event) => setForm({ ...form, note: event.target.value })} /><button className="primary-button" type="submit">Schedule shift</button></div></form><div className="section-heading task-heading"><div><p className="eyebrow">Upcoming</p><h2>{shifts.length} shifts</h2></div></div><div className="record-list">{shifts.map((shift) => <div className="record-row" key={shift.id}><span className="record-avatar"><CalendarDays size={15} /></span><span className="record-copy"><strong>{shift.title}</strong><small>{shift.shift_date} · {shift.starts_at.slice(0, 5)} → {shift.ends_at.slice(0, 5)}</small></span><span className="status-pill">{shift.status}</span></div>)}</div></section>;
}

function ActivityView() {
  const [entries, setEntries] = useState<AuditEntry[]>([]);
  useEffect(() => { void fetch(`${apiBase}/audit-logs`, { credentials: "include" }).then((response) => response.ok ? response.json() : []).then(setEntries).catch(() => setEntries([])); }, []);
  return <section className="content-wrap"><div className="page-heading"><div><p className="eyebrow">Activity</p><h1>Know what changed.</h1><p className="lede">A durable record of privileged actions in this private installation.</p></div></div><div className="section-heading task-heading"><div><p className="eyebrow">Audit trail</p><h2>{entries.length} events</h2></div></div><div className="activity-list">{entries.map((entry) => <div className="activity-row" key={entry.id}><span className="activity-icon"><Activity size={16} /></span><span className="activity-copy"><strong>{entry.action}</strong><small>{entry.actor_name} · {entry.entity_type} · {new Date(entry.created_at).toLocaleString()}</small></span></div>)}</div></section>;
}

type DashboardSummary = { scope: string; open_tasks: number; in_progress_tasks: number; overdue_tasks: number; done_tasks: number; approvals_waiting: number; upcoming_events: number; unread_notifications: number; department_breakdown: Array<{ department: string; open_tasks: number }> };

function DashboardOverview() {
  const [scope, setScope] = useState("organization");
  const [summary, setSummary] = useState<DashboardSummary | null>(null);

  useEffect(() => {
    let active = true;
    void (async () => {
      const response = await fetch(`${apiBase}/dashboard/summary?scope=${scope}`, { credentials: "include" });
      if (response.ok && active) setSummary(await response.json());
    })();
    return () => { active = false; };
  }, [scope]);

  const showBreakdown = (scope === "organization" || scope === "department") && (summary?.department_breakdown?.length ?? 0) > 0;

  return <>
    <div className="section-heading" style={{ marginTop: 34 }}>
      <div><p className="eyebrow">Dashboard</p><h2>Where things stand.</h2></div>
      <select className="scope-select" value={scope} onChange={(event) => setScope(event.target.value)} aria-label="Dashboard scope">
        <option value="own">My work</option>
        <option value="team">My team</option>
        <option value="department">My department</option>
        <option value="organization">Whole company</option>
      </select>
    </div>
    <div className="metric-grid">
      <Metric label="Open work" value={String(summary?.open_tasks ?? 0)} change={`${summary?.in_progress_tasks ?? 0} in progress`} detail="tasks not yet done" />
      <Metric label="Overdue" value={String(summary?.overdue_tasks ?? 0)} change={summary && summary.overdue_tasks > 0 ? "needs attention" : "on track"} detail="past their due date" warning={Boolean(summary && summary.overdue_tasks > 0)} />
      <Metric label="Approvals waiting" value={String(summary?.approvals_waiting ?? 0)} change={`${summary?.upcoming_events ?? 0} events soon`} detail="need your decision" warning={Boolean(summary && summary.approvals_waiting > 0)} />
    </div>
    {showBreakdown && <>
      <div className="section-heading"><div><p className="eyebrow">By department</p><h2>Open work across the company.</h2></div></div>
      <div className="record-list">{summary!.department_breakdown.map((row) => <div className="record-row" key={row.department}><span className="record-avatar department-avatar"><BriefcaseBusiness size={15} /></span><span className="record-copy"><strong>{row.department}</strong><small>{row.open_tasks} open {row.open_tasks === 1 ? "task" : "tasks"}</small></span><span className="status-pill">{row.open_tasks}</span></div>)}</div>
    </>}
  </>;
}

function NotificationBell() {
  const [items, setItems] = useState<NotificationItem[]>([]);
  const [unread, setUnread] = useState(0);
  const [open, setOpen] = useState(false);

  const load = async () => {
    try {
      const response = await fetch(`${apiBase}/notifications`, { credentials: "include" });
      if (!response.ok) return;
      const data = await response.json() as { notifications: NotificationItem[]; unread: number };
      setItems(data.notifications ?? []);
      setUnread(data.unread ?? 0);
    } catch {
      // Offline or signed out; leave the last known state.
    }
  };
  useEffect(() => { void load(); const timer = setInterval(() => void load(), 30000); return () => clearInterval(timer); }, []);

  const markAllRead = async () => {
    await fetch(`${apiBase}/notifications/read`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ all: true }) });
    await load();
  };

  return <div className="notification-bell">
    <button className="search-button" type="button" aria-label={`Notifications, ${unread} unread`} onClick={() => { setOpen((value) => !value); if (!open) void load(); }}>
      <Bell size={16} aria-hidden="true" />
      {unread > 0 && <span className="notification-badge">{unread > 9 ? "9+" : unread}</span>}
    </button>
    {open && <div className="notification-panel">
      <div className="notification-head"><p className="eyebrow">Notifications</p>{unread > 0 && <button className="text-button" type="button" onClick={() => void markAllRead()}>Mark all read</button>}</div>
      {items.length === 0 ? <p className="lede" style={{ fontSize: 12, padding: "8px 0" }}>You are all caught up.</p> : <div className="record-list">{items.slice(0, 12).map((item) => <div className={`notification-row ${item.read_at ? "" : "unread"}`} key={item.id}><strong>{item.title}</strong><small>{item.body} · {new Date(item.created_at).toLocaleString()}</small></div>)}</div>}
    </div>}
  </div>;
}

function ProjectsView() {
  const [projects, setProjects] = useState<ProjectItem[]>([]);
  const [users, setUsers] = useState<Array<{ id: string; display_name: string }>>([]);
  const [form, setForm] = useState({ key: "", name: "", description: "", lead_id: "" });
  const [message, setMessage] = useState("");

  const load = async () => {
    const [projectsResponse, usersResponse] = await Promise.all([
      fetch(`${apiBase}/projects`, { credentials: "include" }),
      fetch(`${apiBase}/users`, { credentials: "include" }),
    ]);
    if (projectsResponse.ok) setProjects(await projectsResponse.json());
    if (usersResponse.ok) setUsers(await usersResponse.json());
  };
  useEffect(() => { void load(); }, []);

  const create = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const response = await fetch(`${apiBase}/projects`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify(form) });
    setMessage(response.ok ? "Project created" : ((await response.json()).error?.message ?? "Could not create project"));
    if (response.ok) { setForm({ key: "", name: "", description: "", lead_id: "" }); await load(); }
  };

  return <section className="content-wrap">
    <div className="page-heading"><div><p className="eyebrow">Work</p><h1>Projects.</h1><p className="lede">Group tasks under the initiatives your teams are delivering.</p></div></div>
    {message && <p className="inline-message">{message}</p>}
    <div className="section-heading task-heading"><div><p className="eyebrow">Portfolio</p><h2>{projects.length} projects</h2></div></div>
    <div className="record-list">{projects.map((project) => <div className="record-row" key={project.id}><span className="record-avatar">{project.key.slice(0, 2)}</span><span className="record-copy"><strong>{project.name}</strong><small>{project.key} · {project.task_count} tasks{project.lead_name ? ` · lead ${project.lead_name}` : ""}</small></span><span className={`status-pill ${project.status === "active" ? "" : "leave-pending"}`}>{project.status.replace("_", " ")}</span></div>)}</div>
    <form className="user-form" onSubmit={create}><p className="eyebrow">New project</p><div className="form-grid"><input placeholder="Key (e.g. MKT)" value={form.key} onChange={(event) => setForm({ ...form, key: event.target.value.toUpperCase().replace(/[^A-Z0-9-]/g, "") })} required /><input placeholder="Project name" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} required /><select value={form.lead_id} onChange={(event) => setForm({ ...form, lead_id: event.target.value })}><option value="">Project lead (optional)</option>{users.map((user) => <option key={user.id} value={user.id}>{user.display_name}</option>)}</select></div><input placeholder="Description (optional)" value={form.description} onChange={(event) => setForm({ ...form, description: event.target.value })} /><button className="primary-button" type="submit">Create project</button></form>
  </section>;
}

function CalendarView() {
  const [events, setEvents] = useState<CalendarEvent[]>([]);
  const [form, setForm] = useState({ title: "", starts_at: "", ends_at: "", all_day: false, visibility: "organization" });
  const [message, setMessage] = useState("");

  const load = async () => {
    const response = await fetch(`${apiBase}/calendar/events`, { credentials: "include" });
    if (response.ok) setEvents(await response.json());
  };
  useEffect(() => { void load(); }, []);

  const create = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const body = { title: form.title, all_day: form.all_day, visibility: form.visibility, starts_at: new Date(form.starts_at).toISOString(), ends_at: new Date(form.ends_at).toISOString() };
    const response = await fetch(`${apiBase}/calendar/events`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
    if (!response.ok) { setMessage((await response.json()).error?.message ?? "Could not create event"); return; }
    const data = await response.json() as { conflict: boolean };
    setMessage(data.conflict ? "Event created · note: it overlaps another of your events" : "Event created");
    setForm({ title: "", starts_at: "", ends_at: "", all_day: false, visibility: "organization" });
    await load();
  };

  return <section className="content-wrap">
    <div className="page-heading"><div><p className="eyebrow">Schedule</p><h1>Calendar.</h1><p className="lede">Shared events for the organization, with conflict detection on your own schedule.</p></div></div>
    {message && <p className="inline-message">{message}</p>}
    <div className="section-heading task-heading"><div><p className="eyebrow">Upcoming</p><h2>{events.length} events</h2></div></div>
    <div className="record-list">{events.map((item) => <div className="record-row" key={item.id}><span className="record-avatar department-avatar"><CalendarDays size={15} /></span><span className="record-copy"><strong>{item.title}</strong><small>{new Date(item.starts_at).toLocaleString()} → {new Date(item.ends_at).toLocaleString()}{item.creator_name ? ` · ${item.creator_name}` : ""}</small></span><span className="status-pill">{item.visibility}</span></div>)}</div>
    <form className="leave-form" onSubmit={create}><p className="eyebrow">New event</p><input placeholder="Event title" value={form.title} onChange={(event) => setForm({ ...form, title: event.target.value })} required /><div className="form-grid"><input type="datetime-local" value={form.starts_at} onChange={(event) => setForm({ ...form, starts_at: event.target.value })} required /><input type="datetime-local" value={form.ends_at} onChange={(event) => setForm({ ...form, ends_at: event.target.value })} required /><select value={form.visibility} onChange={(event) => setForm({ ...form, visibility: event.target.value })}><option value="organization">Organization</option><option value="private">Private</option></select></div><label className="calendar-allday"><input type="checkbox" checked={form.all_day} onChange={(event) => setForm({ ...form, all_day: event.target.checked })} /> All day</label><button className="primary-button" type="submit">Add event</button></form>
  </section>;
}

function WorkflowView() {
  const [inbox, setInbox] = useState<WorkflowInstance[]>([]);
  const [mine, setMine] = useState<WorkflowInstance[]>([]);
  const [definitions, setDefinitions] = useState<WorkflowDefinition[]>([]);
  const [detail, setDetail] = useState<WorkflowInstance | null>(null);
  const [reason, setReason] = useState("");
  const [message, setMessage] = useState("");
  const [request, setRequest] = useState({ definition_id: "", title: "", amount: "" });
  const [showBuilder, setShowBuilder] = useState(false);
  const [builder, setBuilder] = useState<{ code: string; name: string; entity_type: string; steps: WorkflowStepInput[] }>({ code: "", name: "", entity_type: "generic", steps: [{ name: "", approver_role_code: "", required_approvals: 1, min_amount: null, max_amount: null }] });

  const load = async () => {
    const [inboxResponse, mineResponse, definitionsResponse] = await Promise.all([
      fetch(`${apiBase}/workflow/instances?inbox=1`, { credentials: "include" }),
      fetch(`${apiBase}/workflow/instances?mine=1`, { credentials: "include" }),
      fetch(`${apiBase}/workflow/definitions`, { credentials: "include" }),
    ]);
    if (inboxResponse.ok) setInbox(await inboxResponse.json());
    if (mineResponse.ok) setMine(await mineResponse.json());
    if (definitionsResponse.ok) setDefinitions(await definitionsResponse.json());
  };
  useEffect(() => { void load(); }, []);

  const openDetail = async (id: string) => {
    setReason("");
    const response = await fetch(`${apiBase}/workflow/instances/${id}`, { credentials: "include" });
    if (response.ok) setDetail(await response.json());
  };

  const act = async (id: string, action: "approve" | "reject" | "resubmit" | "cancel") => {
    if (action === "reject" && !reason.trim()) { setMessage("A rejection reason is required"); return; }
    const response = await fetch(`${apiBase}/workflow/instances/${id}/${action}`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ reason }) });
    const data = await response.json().catch(() => ({}));
    setMessage(response.ok ? `Request ${data.status ?? action}` : (data.error ?? "Action could not be completed"));
    setReason("");
    await load();
    await openDetail(id);
  };

  const submitRequest = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const amount = request.amount.trim() === "" ? null : Number(request.amount);
    const response = await fetch(`${apiBase}/workflow/instances`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ definition_id: request.definition_id, title: request.title, amount }) });
    const data = await response.json().catch(() => ({}));
    setMessage(response.ok ? "Request submitted for approval" : (data.error ?? "Could not submit request"));
    if (response.ok) { setRequest({ definition_id: "", title: "", amount: "" }); await load(); }
  };

  const createDefinition = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const response = await fetch(`${apiBase}/workflow/definitions`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify(builder) });
    const data = await response.json().catch(() => ({}));
    setMessage(response.ok ? "Workflow created" : (data.error ?? "Could not create workflow"));
    if (response.ok) { setBuilder({ code: "", name: "", entity_type: "generic", steps: [{ name: "", approver_role_code: "", required_approvals: 1, min_amount: null, max_amount: null }] }); setShowBuilder(false); await load(); }
  };

  const updateStep = (index: number, patch: Partial<WorkflowStepInput>) => setBuilder((current) => ({ ...current, steps: current.steps.map((step, position) => position === index ? { ...step, ...patch } : step) }));
  const inboxHas = (id: string) => inbox.some((item) => item.id === id);

  return <section className="content-wrap">
    <div className="page-heading"><div><p className="eyebrow">Governance</p><h1>Approvals & workflows.</h1><p className="lede">Route requests through reviewers. Approvals are always confirmed on the company server.</p></div><button className="primary-button" type="button" onClick={() => setShowBuilder((value) => !value)}>{showBuilder ? "Close builder" : "New workflow"}</button></div>
    {message && <p className="inline-message">{message}</p>}

    <div className="section-heading task-heading"><div><p className="eyebrow">Waiting on you</p><h2>{inbox.length} to review</h2></div></div>
    <div className="record-list">{inbox.length === 0 ? <p className="lede" style={{ padding: "18px 0" }}>Nothing is waiting for your decision.</p> : inbox.map((item) => <button className="record-row workflow-row" key={item.id} type="button" onClick={() => void openDetail(item.id)}><span className="record-avatar"><ClipboardCheck size={15} /></span><span className="record-copy"><strong>{item.title}</strong><small>{item.definition_name} · step: {item.current_step_name || "—"}{item.amount != null ? ` · ${item.amount.toLocaleString()}` : ""} · from {item.submitter_name}</small></span><span className={`status-pill ${workflowStatusClass[item.status] ?? ""}`}>{item.status.replace("_", " ")}</span></button>)}</div>

    <form className="task-composer workflow-composer" onSubmit={submitRequest}>
      <select value={request.definition_id} onChange={(event) => setRequest({ ...request, definition_id: event.target.value })} required><option value="">Select workflow…</option>{definitions.map((definition) => <option key={definition.id} value={definition.id}>{definition.name}</option>)}</select>
      <input placeholder="Request title" value={request.title} onChange={(event) => setRequest({ ...request, title: event.target.value })} required />
      <input placeholder="Amount (optional)" inputMode="decimal" value={request.amount} onChange={(event) => setRequest({ ...request, amount: event.target.value.replace(/[^0-9.]/g, "") })} />
      <button className="primary-button" type="submit">Submit</button>
    </form>

    <div className="section-heading task-heading"><div><p className="eyebrow">Your requests</p><h2>{mine.length} submitted</h2></div></div>
    <div className="record-list">{mine.map((item) => <button className="record-row workflow-row" key={item.id} type="button" onClick={() => void openDetail(item.id)}><span className="record-avatar department-avatar"><ClipboardCheck size={15} /></span><span className="record-copy"><strong>{item.title}</strong><small>{item.definition_name}{item.current_step_name ? ` · at: ${item.current_step_name}` : ""} · updated {new Date(item.updated_at).toLocaleDateString()}</small></span><span className={`status-pill ${workflowStatusClass[item.status] ?? ""}`}>{item.status.replace("_", " ")}</span></button>)}</div>

    {detail && <div className="rbac-panel">
      <div className="section-heading"><div><p className="eyebrow">{detail.definition_name}</p><h2>{detail.title}</h2></div><button className="text-button muted" type="button" onClick={() => setDetail(null)}>Close</button></div>
      <p className="lede" style={{ marginBottom: 18 }}>Status: <span className={`status-pill ${workflowStatusClass[detail.status] ?? ""}`}>{detail.status.replace("_", " ")}</span>{detail.current_step_name ? ` · current step: ${detail.current_step_name}` : ""}{detail.amount != null ? ` · amount ${detail.amount.toLocaleString()}` : ""}</p>
      <div className="comment-thread">{(detail.actions ?? []).map((action) => <p key={action.id}><strong>{action.actor_name}</strong> {action.action}{action.step_order ? ` · step ${action.step_order}` : ""}{action.reason ? ` — “${action.reason}”` : ""} <small style={{ color: "var(--faint)" }}>· {new Date(action.created_at).toLocaleString()}</small></p>)}</div>
      {(inboxHas(detail.id) || (detail.status === "rejected" && detail.submitter_name)) && <div className="workflow-actions">
        <input placeholder="Reason (required to reject)" value={reason} onChange={(event) => setReason(event.target.value)} />
        {inboxHas(detail.id) && <><button className="primary-button" type="button" onClick={() => void act(detail.id, "approve")}>Approve</button><button className="text-button" type="button" onClick={() => void act(detail.id, "reject")}>Reject</button></>}
        {detail.status === "rejected" && <button className="text-button" type="button" onClick={() => void act(detail.id, "resubmit")}>Resubmit</button>}
        {(detail.status === "in_review" || detail.status === "draft") && <button className="text-button muted" type="button" onClick={() => void act(detail.id, "cancel")}>Cancel request</button>}
      </div>}
    </div>}

    {showBuilder && <form className="user-form" onSubmit={createDefinition}>
      <p className="eyebrow">New workflow definition</p>
      <div className="form-grid"><input placeholder="Code (e.g. expense-approval)" value={builder.code} onChange={(event) => setBuilder({ ...builder, code: event.target.value.toLowerCase().replace(/[^a-z0-9._-]/g, "-") })} required /><input placeholder="Name" value={builder.name} onChange={(event) => setBuilder({ ...builder, name: event.target.value })} required /><input placeholder="Entity type (generic)" value={builder.entity_type} onChange={(event) => setBuilder({ ...builder, entity_type: event.target.value })} /></div>
      <p className="eyebrow" style={{ marginTop: 20 }}>Approval steps</p>
      {builder.steps.map((step, index) => <div className="workflow-step-row" key={index}>
        <input placeholder={`Step ${index + 1} name`} value={step.name} onChange={(event) => updateStep(index, { name: event.target.value })} required />
        <input placeholder="Approver role code (blank = any)" value={step.approver_role_code} onChange={(event) => updateStep(index, { approver_role_code: event.target.value.toLowerCase().replace(/[^a-z0-9._-]/g, "-") })} />
        <input placeholder="# approvals" inputMode="numeric" value={String(step.required_approvals)} onChange={(event) => updateStep(index, { required_approvals: Math.max(1, Number(event.target.value.replace(/[^0-9]/g, "")) || 1) })} />
        <input placeholder="Min amount" inputMode="decimal" value={step.min_amount ?? ""} onChange={(event) => updateStep(index, { min_amount: event.target.value.trim() === "" ? null : Number(event.target.value.replace(/[^0-9.]/g, "")) })} />
        <input placeholder="Max amount" inputMode="decimal" value={step.max_amount ?? ""} onChange={(event) => updateStep(index, { max_amount: event.target.value.trim() === "" ? null : Number(event.target.value.replace(/[^0-9.]/g, "")) })} />
        {builder.steps.length > 1 && <button className="text-button muted" type="button" onClick={() => setBuilder((current) => ({ ...current, steps: current.steps.filter((_, position) => position !== index) }))}>Remove</button>}
      </div>)}
      <button className="text-button" type="button" onClick={() => setBuilder((current) => ({ ...current, steps: [...current.steps, { name: "", approver_role_code: "", required_approvals: 1, min_amount: null, max_amount: null }] }))}>Add step <ArrowUpRight size={15} /></button>
      <button className="primary-button" type="submit">Create workflow</button>
    </form>}
  </section>;
}

function PeopleView() {
  const [users, setUsers] = useState<UserRecord[]>([]);
  const [departments, setDepartments] = useState<DepartmentRecord[]>([]);
  const [roles, setRoles] = useState<RoleRecord[]>([]);
  const [permissions, setPermissions] = useState<PermissionRecord[]>([]);
  const [roleForm, setRoleForm] = useState({ code: "", name: "", permissions: [] as string[] });
  const [assignment, setAssignment] = useState({ user_id: "", role_id: "" });
  const [showUserForm, setShowUserForm] = useState(false);
  const [userForm, setUserForm] = useState({ display_name: "", email: "", password: "" });
  const [departmentForm, setDepartmentForm] = useState({ name: "", slug: "" });
  const [message, setMessage] = useState("");

  const load = async () => {
    const [usersResponse, departmentsResponse, rolesResponse, permissionsResponse] = await Promise.all([
      fetch(`${apiBase}/users`, { credentials: "include" }),
      fetch(`${apiBase}/departments`, { credentials: "include" }),
      fetch(`${apiBase}/roles`, { credentials: "include" }),
      fetch(`${apiBase}/permissions`, { credentials: "include" }),
    ]);
    if (usersResponse.ok) setUsers(await usersResponse.json());
    if (departmentsResponse.ok) setDepartments(await departmentsResponse.json());
    if (rolesResponse.ok) setRoles(await rolesResponse.json());
    if (permissionsResponse.ok) setPermissions(await permissionsResponse.json());
  };

  useEffect(() => { void load(); }, []);

  const createUser = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setMessage("");
    const response = await fetch(`${apiBase}/users`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify(userForm) });
    if (!response.ok) { setMessage((await response.json()).error ?? "Could not create user"); return; }
    setUserForm({ display_name: "", email: "", password: "" });
    setShowUserForm(false);
    setMessage("User added");
    await load();
  };

  const createDepartment = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setMessage("");
    const response = await fetch(`${apiBase}/departments`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify(departmentForm) });
    if (!response.ok) { setMessage((await response.json()).error ?? "Could not create department"); return; }
    setDepartmentForm({ name: "", slug: "" });
    setMessage("Department added");
    await load();
  };

  const createRole = async (event: FormEvent<HTMLFormElement>) => { event.preventDefault(); const response = await fetch(`${apiBase}/roles`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify(roleForm) }); if (!response.ok) { setMessage("Role could not be created"); return; } setRoleForm({ code: "", name: "", permissions: [] }); setMessage("Role created"); await load(); };
  const assignRole = async (event: FormEvent<HTMLFormElement>) => { event.preventDefault(); const response = await fetch(`${apiBase}/user-roles`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify(assignment) }); setMessage(response.ok ? "Role assigned" : "Role could not be assigned"); };

  const resetPassword = async (user: UserRecord) => {
    const newPassword = window.prompt(`Set a new temporary password for ${user.display_name} (12+ characters). Their sessions will be signed out.`);
    if (!newPassword) return;
    const response = await fetch(`${apiBase}/users/${user.id}/reset-password`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ new_password: newPassword }) });
    setMessage(response.ok ? `Password reset for ${user.display_name}` : ((await response.json()).error ?? "Could not reset password"));
  };
  const offboard = async (user: UserRecord) => {
    if (!window.confirm(`Offboard ${user.display_name}? This revokes their sessions, roles, and MFA immediately.`)) return;
    const response = await fetch(`${apiBase}/users/${user.id}/offboard`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify({}) });
    setMessage(response.ok ? `${user.display_name} offboarded` : ((await response.json()).error ?? "Could not offboard"));
    if (response.ok) await load();
  };

  return <section className="content-wrap">
    <div className="page-heading"><div><p className="eyebrow">Organization</p><h1>People & teams.</h1><p className="lede">Manage the people and departments in this private installation.</p></div><button className="primary-button" type="button" onClick={() => setShowUserForm((value) => !value)}>{showUserForm ? "Close form" : "Add person"}</button></div>
    {message && <p className="inline-message">{message}</p>}
    <div className="people-grid">
      <div className="people-section"><div className="section-heading"><div><p className="eyebrow">Directory</p><h2>{users.length} people</h2></div></div><div className="record-list">{users.map((user) => <div className="record-row" key={user.id}><span className="record-avatar">{user.display_name.slice(0, 1).toUpperCase()}</span><span className="record-copy"><strong>{user.display_name}</strong><small>{user.email}</small></span><span className={`status-pill ${user.status === "offboarded" ? "leave-rejected" : ""}`}>{user.status}</span>{user.status !== "offboarded" && <span className="record-actions"><button className="text-button muted" type="button" onClick={() => void resetPassword(user)}>Reset</button><button className="text-button muted" type="button" onClick={() => void offboard(user)}>Offboard</button></span>}</div>)}</div></div>
      <div className="people-section"><div className="section-heading"><div><p className="eyebrow">Structure</p><h2>{departments.length} departments</h2></div></div><form className="compact-form" onSubmit={createDepartment}><input value={departmentForm.name} onChange={(event) => setDepartmentForm({ ...departmentForm, name: event.target.value })} placeholder="Department name" required /><input value={departmentForm.slug} onChange={(event) => setDepartmentForm({ ...departmentForm, slug: event.target.value.toLowerCase().replace(/[^a-z0-9-]/g, "-") })} placeholder="slug" required /><button className="text-button" type="submit">Add department <ArrowUpRight size={15} /></button></form><div className="record-list">{departments.map((department) => <div className="record-row" key={department.id}><span className="record-avatar department-avatar"><BriefcaseBusiness size={15} /></span><span className="record-copy"><strong>{department.name}</strong><small>{department.slug}</small></span></div>)}</div></div>
    </div>
    {showUserForm && <form className="user-form" onSubmit={createUser}><p className="eyebrow">New person</p><div className="form-grid"><input value={userForm.display_name} onChange={(event) => setUserForm({ ...userForm, display_name: event.target.value })} placeholder="Full name" required /><input type="email" value={userForm.email} onChange={(event) => setUserForm({ ...userForm, email: event.target.value })} placeholder="Email" required /><input type="password" value={userForm.password} onChange={(event) => setUserForm({ ...userForm, password: event.target.value })} placeholder="Temporary password (12+ characters)" minLength={12} required /></div><button className="primary-button" type="submit">Create person</button></form>}
    {roles.length > 0 && <div className="rbac-panel"><div className="section-heading"><div><p className="eyebrow">Access control</p><h2>{roles.length} roles</h2></div></div><div className="record-list">{roles.map((role) => <div className="record-row" key={role.id}><span className="record-avatar">R</span><span className="record-copy"><strong>{role.name}</strong><small>{role.code} · {role.permissions.length} permissions</small></span></div>)}</div><form className="compact-form role-form" onSubmit={createRole}><p className="eyebrow">Create role</p><div className="form-grid"><input placeholder="Role code" value={roleForm.code} onChange={(event) => setRoleForm({ ...roleForm, code: event.target.value.toLowerCase().replace(/[^a-z0-9._-]/g, "-") })} required /><input placeholder="Role name" value={roleForm.name} onChange={(event) => setRoleForm({ ...roleForm, name: event.target.value })} required /></div><div className="permission-grid">{permissions.map((permission) => <label key={permission.code}><input type="checkbox" checked={roleForm.permissions.includes(permission.code)} onChange={(event) => setRoleForm({ ...roleForm, permissions: event.target.checked ? [...roleForm.permissions, permission.code] : roleForm.permissions.filter((code) => code !== permission.code) })} />{permission.code}</label>)}</div><button className="primary-button" type="submit">Create role</button></form><form className="compact-form role-form" onSubmit={assignRole}><p className="eyebrow">Assign role</p><div className="form-grid"><select value={assignment.user_id} onChange={(event) => setAssignment({ ...assignment, user_id: event.target.value })} required><option value="">Select person</option>{users.map((user) => <option key={user.id} value={user.id}>{user.display_name}</option>)}</select><select value={assignment.role_id} onChange={(event) => setAssignment({ ...assignment, role_id: event.target.value })} required><option value="">Select role</option>{roles.map((role) => <option key={role.id} value={role.id}>{role.name}</option>)}</select></div><button className="text-button" type="submit">Assign role <ArrowUpRight size={15} /></button></form></div>}
    <SecurityPanel />
  </section>;
}

function SecurityPanel() {
  const [passwordForm, setPasswordForm] = useState({ current_password: "", new_password: "" });
  const [enrollment, setEnrollment] = useState<{ secret: string; otpauth_uri: string } | null>(null);
  const [mfaCode, setMfaCode] = useState("");
  const [message, setMessage] = useState("");

  const changePassword = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const response = await fetch(`${apiBase}/auth/change-password`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify(passwordForm) });
    setMessage(response.ok ? "Password changed · other sessions signed out" : ((await response.json()).error ?? "Could not change password"));
    if (response.ok) setPasswordForm({ current_password: "", new_password: "" });
  };
  const startEnrollment = async () => {
    const response = await fetch(`${apiBase}/auth/mfa/enroll`, { method: "POST", credentials: "include" });
    if (response.ok) { setEnrollment(await response.json()); setMessage("Scan the secret in your authenticator app, then enter a code to confirm."); } else setMessage("Could not start MFA enrollment");
  };
  const verifyEnrollment = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const response = await fetch(`${apiBase}/auth/mfa/verify`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ code: mfaCode }) });
    setMessage(response.ok ? "Two-factor authentication enabled" : ((await response.json()).error ?? "Invalid code"));
    if (response.ok) { setEnrollment(null); setMfaCode(""); }
  };

  return <div className="rbac-panel">
    <div className="section-heading"><div><p className="eyebrow">Your account security</p><h2>Password & two-factor</h2></div></div>
    {message && <p className="inline-message">{message}</p>}
    <form className="compact-form role-form" onSubmit={changePassword}><p className="eyebrow">Change your password</p><div className="form-grid"><input type="password" placeholder="Current password" value={passwordForm.current_password} onChange={(event) => setPasswordForm({ ...passwordForm, current_password: event.target.value })} required /><input type="password" placeholder="New password (12+ characters)" minLength={12} value={passwordForm.new_password} onChange={(event) => setPasswordForm({ ...passwordForm, new_password: event.target.value })} required /></div><button className="primary-button" type="submit">Update password</button></form>
    <div className="role-form">
      <p className="eyebrow">Two-factor authentication</p>
      {!enrollment ? <button className="text-button" type="button" onClick={() => void startEnrollment()}>Enable authenticator app <ArrowUpRight size={15} /></button> : <form className="compact-form" onSubmit={verifyEnrollment}><p className="lede" style={{ fontSize: 12 }}>Secret: <code>{enrollment.secret}</code></p><input inputMode="numeric" placeholder="6-digit code" value={mfaCode} onChange={(event) => setMfaCode(event.target.value.replace(/[^0-9]/g, "").slice(0, 6))} required /><button className="primary-button" type="submit">Confirm & enable</button></form>}
    </div>
  </div>;
}

function LoadingScreen() {
  return <main className="auth-shell"><div className="auth-panel"><div className="brand-mark">N</div><p className="eyebrow">Name</p><h1>Preparing your workspace.</h1><p className="lede">Checking the local application server.</p></div></main>;
}

function AuthScreen({ mode, theme, onThemeToggle, onAuthenticated }: { mode: "setup" | "login"; theme: Theme; onThemeToggle: () => void; onAuthenticated: (user: CurrentUser) => void }) {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [name, setName] = useState("");
  const [organizationName, setOrganizationName] = useState("");
  const [organizationSlug, setOrganizationSlug] = useState("");
  const [code, setCode] = useState("");
  const [mfaRequired, setMfaRequired] = useState(false);
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setError("");
    setSubmitting(true);
    try {
      if (mode === "setup") {
        const setupResponse = await fetch(`${apiBase}/setup`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ organization_name: organizationName, organization_slug: organizationSlug, name, email, password }) });
        if (!setupResponse.ok) throw new Error((await setupResponse.json()).error ?? "Could not complete setup");
      }

      const loginResponse = await fetch(`${apiBase}/auth/login`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ email, password, code }) });
      if (!loginResponse.ok) {
        const body = await loginResponse.json();
        if (body.mfa_required) {
          setMfaRequired(true);
          setError(code ? "Invalid verification code" : "Enter the code from your authenticator app");
          return;
        }
        throw new Error(body.error ?? "Could not sign in");
      }
      const meResponse = await fetch(`${apiBase}/auth/me`, { credentials: "include" });
      if (!meResponse.ok) throw new Error("Could not load your account");
      onAuthenticated(await meResponse.json());
    } catch (submissionError) {
      setError(submissionError instanceof Error ? submissionError.message : "Something went wrong");
    } finally {
      setSubmitting(false);
    }
  };

  return <main className="auth-shell">
    <section className="auth-panel">
      <div className="auth-heading"><div className="brand-mark">N</div><p className="eyebrow">Name · Private workspace</p><h1>{mode === "setup" ? "Set up your company." : "Welcome back."}</h1><p className="lede">{mode === "setup" ? "Create the first owner account for this installation." : "Sign in to continue to your workspace."}</p></div>
      <form className="auth-form" onSubmit={submit}>
        {mode === "setup" && <><label>Company name<input value={organizationName} onChange={(event) => setOrganizationName(event.target.value)} placeholder="Acme Company" required /></label><label>Company slug<input value={organizationSlug} onChange={(event) => setOrganizationSlug(event.target.value.toLowerCase().replace(/[^a-z0-9-]/g, "-"))} placeholder="acme-company" required /></label><label>Your name<input value={name} onChange={(event) => setName(event.target.value)} placeholder="Lucas Marshall" required /></label></>}
        <label>Email<input type="email" value={email} onChange={(event) => setEmail(event.target.value)} placeholder="you@company.com" required /></label>
        <label>Password<input type="password" value={password} onChange={(event) => setPassword(event.target.value)} placeholder="At least 12 characters" minLength={12} required /></label>
        {mfaRequired && <label>Verification code<input inputMode="numeric" autoComplete="one-time-code" value={code} onChange={(event) => setCode(event.target.value.replace(/[^0-9]/g, "").slice(0, 6))} placeholder="6-digit code" required /></label>}
        {error && <p className="form-error" role="alert">{error}</p>}
        <button className="primary-button submit-button" type="submit" disabled={submitting}>{submitting ? "Connecting…" : mode === "setup" ? "Create workspace" : mfaRequired ? "Verify & sign in" : "Sign in"}</button>
      </form>
      <p className="auth-footnote">Your data stays on this company's private installation.</p>
    </section>
    <button className="theme-switcher" type="button" onClick={onThemeToggle} aria-label={`Switch to ${theme === "dark" ? "light" : "dark"} theme`}>{theme === "dark" ? <Sun size={17} /> : <Moon size={17} />}<span>{theme === "dark" ? "Light" : "Dark"}</span></button>
  </main>;
}

function Metric({ label, value, change, detail, warning = false }: { label: string; value: string; change: string; detail: string; warning?: boolean }) {
  return <div className="metric-row">
    <div><p className="metric-label">{label}</p><strong className="metric-value">{value}</strong></div>
    <div className={`metric-change ${warning ? "warning" : ""}`}>{change}<small>{detail}</small></div>
  </div>;
}

export default App;
