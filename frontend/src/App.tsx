import { useEffect, useMemo, useRef, useState, type FormEvent, type KeyboardEvent as ReactKeyboardEvent } from "react";
import {
  Activity,
  ArrowUpRight,
  Bell,
  BriefcaseBusiness,
  Check,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  CircleHelp,
  ClipboardCheck,
  Clock3,
  CalendarDays,
  FolderKanban,
  IdCard,
  LayoutDashboard,
  LifeBuoy,
  LineChart,
  Moon,
  PanelLeft,
  Plus,
  Search,
  SlidersHorizontal,
  Sun,
  Target,
  Trash2,
  UsersRound,
  Wallet,
} from "lucide-react";
import { cacheAttendance, cacheLeave, cacheSession, cacheShifts, cacheTasks, deleteLocalTask, discardOperation, getCachedSession, getLocalAttendance, getLocalLeave, getLocalShifts, getLocalTasks, getOutboxStatuses, initializeLocalDb, pullRemoteChanges, queueOperation, retryOperation, syncPendingOperations, type LocalAttendance, type LocalLeave, type LocalShift, type LocalTask } from "./localDb";

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

type VendorItem = { id: string; name: string; contact_email: string; contact_phone: string; status: string };
type ExpenseItem = { id: string; description: string; category: string; amount: number; currency: string; status: string; vendor_id: string; vendor_name: string; submitter_name: string; approval_status: string; created_at: string };
type NotificationItem = { id: string; kind: string; title: string; body: string; entity_type: string; entity_id: string; read_at: string | null; created_at: string };
type ProjectItem = { id: string; key: string; name: string; description: string; status: string; lead_id: string; lead_name: string; task_count: number; created_at: string };
type CalendarEvent = { id: string; title: string; description: string; starts_at: string; ends_at: string; all_day: boolean; visibility: string; creator_name: string };
type LookupOption = { id: string; category: string; value: string; label: string; color: string; sort_order: number; is_active: boolean };
type LookupCatalogItem = { category: string; title: string; permission: string; editable: boolean };
type DropdownOption = { value: string; label: string; color?: string };

const workflowStatusClass: Record<string, string> = { in_review: "leave-pending", approved: "leave-approved", rejected: "leave-rejected", cancelled: "leave-rejected", draft: "" };

const viewForLabel = (label: string): string => label === "People" ? "people" : label === "Work" ? "work" : label === "Projects" ? "projects" : label === "Approvals" ? "approvals" : label === "Finance" ? "finance" : label === "HR" ? "hr" : label === "Sales" ? "sales" : label === "IT" ? "it" : label === "Reports" ? "reports" : label === "Calendar" ? "calendar" : label === "Attendance" ? "attendance" : label === "Leave" ? "leave" : label === "Schedule" ? "schedule" : label === "Activity" ? "activity" : label === "Settings" ? "settings" : "overview";

const apiBase = "http://localhost:8080/api/v1";

const navigation = [
  { label: "Overview", icon: LayoutDashboard, active: true },
  { label: "Work", icon: BriefcaseBusiness, permission: "tasks.read" },
  { label: "Projects", icon: FolderKanban, permission: "projects.read" },
  { label: "Approvals", icon: ClipboardCheck, permission: "workflow.read" },
  { label: "Finance", icon: Wallet, permission: "finance.read" },
  { label: "HR", icon: IdCard, permission: "hr.read" },
  { label: "Sales", icon: Target, permission: "sales.read" },
  { label: "IT", icon: LifeBuoy, permission: "itops.read" },
  { label: "Reports", icon: LineChart, permission: "analytics.read" },
  { label: "Calendar", icon: CalendarDays, permission: "calendar.read" },
  { label: "Attendance", icon: Clock3, permission: "attendance.read" },
  { label: "Leave", icon: CalendarDays, permission: "leave.read" },
  { label: "Schedule", icon: CalendarDays, permission: "shifts.read" },
  { label: "People", icon: UsersRound, permission: "users.read" },
  { label: "Activity", icon: Activity, permission: "organization.read" },
  { label: "Settings", icon: SlidersHorizontal },
];

// Dropdown is a fully custom, accessible replacement for native <select>.
// Keyboard: Up/Down move, Enter/Space select, Escape closes, Home/End jump.
// Colored swatches surface option colors (used for leave types, statuses, etc.).
function Dropdown({ value, options, onChange, placeholder = "Select…", ariaLabel, disabled = false }: {
  value: string;
  options: DropdownOption[];
  onChange: (value: string) => void;
  placeholder?: string;
  ariaLabel?: string;
  disabled?: boolean;
}) {
  const [open, setOpen] = useState(false);
  const [activeIndex, setActiveIndex] = useState(-1);
  const rootRef = useRef<HTMLDivElement>(null);
  const menuRef = useRef<HTMLUListElement>(null);
  const selected = options.find((option) => option.value === value);

  useEffect(() => {
    if (!open) return;
    const onDocClick = (event: MouseEvent) => {
      if (rootRef.current && !rootRef.current.contains(event.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onDocClick);
    return () => document.removeEventListener("mousedown", onDocClick);
  }, [open]);

  useEffect(() => {
    if (!open || activeIndex < 0) return;
    (menuRef.current?.children[activeIndex] as HTMLElement | undefined)?.scrollIntoView({ block: "nearest" });
  }, [open, activeIndex]);

  useEffect(() => {
    if (open) setActiveIndex(Math.max(0, options.findIndex((option) => option.value === value)));
  }, [open, value, options]);

  const commit = (index: number) => {
    const option = options[index];
    if (option) onChange(option.value);
    setOpen(false);
  };

  const onKeyDown = (event: ReactKeyboardEvent) => {
    if (disabled) return;
    if (!open && (event.key === "ArrowDown" || event.key === "Enter" || event.key === " ")) {
      event.preventDefault();
      setOpen(true);
      return;
    }
    if (!open) return;
    if (event.key === "ArrowDown") { event.preventDefault(); setActiveIndex((index) => Math.min(options.length - 1, index + 1)); }
    else if (event.key === "ArrowUp") { event.preventDefault(); setActiveIndex((index) => Math.max(0, index - 1)); }
    else if (event.key === "Home") { event.preventDefault(); setActiveIndex(0); }
    else if (event.key === "End") { event.preventDefault(); setActiveIndex(options.length - 1); }
    else if (event.key === "Enter" || event.key === " ") { event.preventDefault(); commit(activeIndex); }
    else if (event.key === "Escape") { event.preventDefault(); setOpen(false); }
  };

  return (
    <div className={`dropdown ${disabled ? "disabled" : ""}`} ref={rootRef}>
      <button type="button" className={`dropdown-trigger ${open ? "open" : ""}`} aria-haspopup="listbox" aria-expanded={open} aria-label={ariaLabel} disabled={disabled} onClick={() => !disabled && setOpen((value) => !value)} onKeyDown={onKeyDown}>
        {selected?.color && <span className="dropdown-swatch" style={{ background: selected.color }} />}
        <span className={selected ? "dropdown-value" : "dropdown-placeholder"}>{selected ? selected.label : placeholder}</span>
        <ChevronDown size={15} className="dropdown-caret" aria-hidden="true" />
      </button>
      {open && (
        <ul className="dropdown-menu" role="listbox" aria-label={ariaLabel} tabIndex={-1} ref={menuRef}>
          {options.length === 0 && <li className="dropdown-empty">No options</li>}
          {options.map((option, index) => (
            <li
              key={option.value}
              role="option"
              aria-selected={option.value === value}
              className={`dropdown-option ${index === activeIndex ? "active" : ""} ${option.value === value ? "selected" : ""}`}
              onMouseEnter={() => setActiveIndex(index)}
              onMouseDown={(event) => { event.preventDefault(); commit(index); }}
            >
              {option.color && <span className="dropdown-swatch" style={{ background: option.color }} />}
              <span className="dropdown-option-label">{option.label}</span>
              {option.value === value && <Check size={14} className="dropdown-check" aria-hidden="true" />}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

// useLookup loads an admin-managed option list (leave types, categories, …)
// from the backend so dropdown contents are configurable rather than hardcoded.
function useLookup(category: string): DropdownOption[] {
  const [options, setOptions] = useState<DropdownOption[]>([]);
  useEffect(() => {
    let alive = true;
    fetch(`${apiBase}/lookups/${category}`, { credentials: "include" })
      .then((response) => (response.ok ? response.json() : null))
      .then((data: { options: LookupOption[] } | null) => {
        if (alive && data) setOptions(data.options.map((option) => ({ value: option.value, label: option.label, color: option.color || undefined })));
      })
      .catch(() => { /* offline or unauthorized — dropdown simply stays empty */ });
    return () => { alive = false; };
  }, [category]);
  return options;
}

// Shared option sets for the custom Dropdown, replacing inline <option> lists.
const PRIORITY_OPTIONS: DropdownOption[] = [{ value: "low", label: "Low priority", color: "#777975" }, { value: "normal", label: "Normal priority", color: "#3b6ea5" }, { value: "high", label: "High priority", color: "#aa6c29" }, { value: "urgent", label: "Urgent", color: "#b3453b" }];
const RECURRENCE_OPTIONS: DropdownOption[] = [{ value: "", label: "No repeat" }, { value: "daily", label: "Daily" }, { value: "weekly", label: "Weekly" }, { value: "monthly", label: "Monthly" }];
const TASK_SCOPE_OPTIONS: DropdownOption[] = [{ value: "own", label: "My tasks" }, { value: "team", label: "Team tasks" }, { value: "department", label: "Department tasks" }, { value: "organization", label: "All company tasks" }];
const DASH_SCOPE_OPTIONS: DropdownOption[] = [{ value: "own", label: "My work" }, { value: "team", label: "My team" }, { value: "department", label: "My department" }, { value: "organization", label: "Whole company" }];
const PERIOD_OPTIONS: DropdownOption[] = [{ value: "7d", label: "Last 7 days" }, { value: "30d", label: "Last 30 days" }, { value: "90d", label: "Last 90 days" }, { value: "ytd", label: "Year to date" }];
const PERIOD_SHORT_OPTIONS: DropdownOption[] = [{ value: "7d", label: "7 days" }, { value: "30d", label: "30 days" }, { value: "90d", label: "90 days" }, { value: "ytd", label: "Year to date" }];
const REPORT_TYPE_OPTIONS: DropdownOption[] = [{ value: "spending", label: "Spending" }, { value: "attendance", label: "Attendance" }, { value: "sales", label: "Sales" }, { value: "custom", label: "Custom" }];
const CADENCE_OPTIONS: DropdownOption[] = [{ value: "daily", label: "Daily" }, { value: "weekly", label: "Weekly" }, { value: "monthly", label: "Monthly" }];
const EMPLOYMENT_OPTIONS: DropdownOption[] = [{ value: "full_time", label: "Full time" }, { value: "part_time", label: "Part time" }, { value: "contract", label: "Contract" }, { value: "intern", label: "Intern" }];
const DOC_TYPE_OPTIONS: DropdownOption[] = [{ value: "contract", label: "Contract" }, { value: "id", label: "ID" }, { value: "review", label: "Review" }, { value: "general", label: "General" }];
const LEAD_SOURCE_OPTIONS: DropdownOption[] = [{ value: "website", label: "Website" }, { value: "referral", label: "Referral" }, { value: "event", label: "Event" }, { value: "other", label: "Other" }];
const LEAD_STATUS_OPTIONS: DropdownOption[] = [{ value: "new", label: "New" }, { value: "qualified", label: "Qualified" }, { value: "converted", label: "Converted" }, { value: "lost", label: "Lost" }];
const OPP_STAGE_OPTIONS: DropdownOption[] = [{ value: "prospect", label: "Prospect" }, { value: "qualified", label: "Qualified" }, { value: "proposal", label: "Proposal" }, { value: "won", label: "Won" }, { value: "lost", label: "Lost" }];
const TICKET_TYPE_OPTIONS: DropdownOption[] = [{ value: "ticket", label: "Ticket" }, { value: "service_request", label: "Service request" }, { value: "access_request", label: "Access request" }, { value: "incident", label: "Incident" }];
const TICKET_PRIORITY_OPTIONS: DropdownOption[] = [{ value: "low", label: "Low", color: "#777975" }, { value: "normal", label: "Normal", color: "#3b6ea5" }, { value: "high", label: "High", color: "#aa6c29" }, { value: "urgent", label: "Urgent", color: "#b3453b" }];
const TICKET_STATUS_OPTIONS: DropdownOption[] = [{ value: "open", label: "Open" }, { value: "in_progress", label: "In progress" }, { value: "resolved", label: "Resolved" }, { value: "closed", label: "Closed" }];
const ASSET_CATEGORY_OPTIONS: DropdownOption[] = [{ value: "hardware", label: "Hardware" }, { value: "software", label: "Software" }, { value: "license", label: "License" }];

// DatePicker: a custom popover calendar replacing native <input type="date">.
// Value is an ISO "YYYY-MM-DD" string, matching the previous inputs.
function DatePicker({ value, onChange, ariaLabel, placeholder = "Select date" }: { value: string; onChange: (value: string) => void; ariaLabel?: string; placeholder?: string }) {
  const [open, setOpen] = useState(false);
  const [view, setView] = useState<Date>(() => (value ? new Date(`${value}T00:00`) : startOfDay(new Date())));
  const rootRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const onDocClick = (event: MouseEvent) => { if (rootRef.current && !rootRef.current.contains(event.target as Node)) setOpen(false); };
    document.addEventListener("mousedown", onDocClick);
    return () => document.removeEventListener("mousedown", onDocClick);
  }, [open]);
  useEffect(() => { if (open && value) setView(new Date(`${value}T00:00`)); }, [open, value]);
  const selected = value ? new Date(`${value}T00:00`) : null;
  const label = selected ? `${MONTHS[selected.getMonth()].slice(0, 3)} ${selected.getDate()}, ${selected.getFullYear()}` : placeholder;
  const days = useMemo(() => { const first = new Date(view.getFullYear(), view.getMonth(), 1); const gridStart = startOfWeek(first); return Array.from({ length: 42 }, (_, i) => addDays(gridStart, i)); }, [view]);
  const todayDate = startOfDay(new Date());
  return <div className="datepicker" ref={rootRef}>
    <button type="button" className={`dropdown-trigger ${open ? "open" : ""}`} aria-haspopup="dialog" aria-expanded={open} aria-label={ariaLabel} onClick={() => setOpen((current) => !current)}>
      <CalendarDays size={14} className="dp-icon" aria-hidden="true" />
      <span className={selected ? "dropdown-value" : "dropdown-placeholder"}>{label}</span>
      <ChevronDown size={15} className="dropdown-caret" aria-hidden="true" />
    </button>
    {open && <div className="dp-pop" role="dialog">
      <div className="dp-head">
        <button type="button" className="cal-arrow" aria-label="Previous month" onClick={() => setView((current) => new Date(current.getFullYear(), current.getMonth() - 1, 1))}><ChevronLeft size={15} /></button>
        <strong>{MONTHS[view.getMonth()]} {view.getFullYear()}</strong>
        <button type="button" className="cal-arrow" aria-label="Next month" onClick={() => setView((current) => new Date(current.getFullYear(), current.getMonth() + 1, 1))}><ChevronRight size={15} /></button>
      </div>
      <div className="dp-weekdays">{["M", "T", "W", "T", "F", "S", "S"].map((weekday, index) => <span key={index}>{weekday}</span>)}</div>
      <div className="dp-grid">{days.map((day) => {
        const isSelected = selected && sameDay(day, selected);
        const isToday = sameDay(day, todayDate);
        const outside = day.getMonth() !== view.getMonth();
        return <button type="button" key={dateKey(day)} className={`dp-day ${isSelected ? "sel" : ""} ${isToday ? "today" : ""} ${outside ? "outside" : ""}`} onClick={() => { onChange(dateKey(day)); setOpen(false); }}>{day.getDate()}</button>;
      })}</div>
      <div className="dp-foot"><button type="button" className="dp-clear" onClick={() => { onChange(dateKey(new Date())); setOpen(false); }}>Today</button></div>
    </div>}
  </div>;
}

// TimePicker: a custom dropdown of 15-minute slots replacing <input type="time">.
function TimePicker({ value, onChange, ariaLabel }: { value: string; onChange: (value: string) => void; ariaLabel?: string }) {
  const options = useMemo(() => Array.from({ length: 24 * 4 }, (_, i) => { const hour = Math.floor(i / 4); const minute = (i % 4) * 15; const slot = `${String(hour).padStart(2, "0")}:${String(minute).padStart(2, "0")}`; return { value: slot, label: slot }; }), []);
  return <Dropdown value={value} options={options} onChange={onChange} ariaLabel={ariaLabel} placeholder="--:--" />;
}

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

        {activeView === "people" ? <PeopleView /> : activeView === "work" ? <WorkView /> : activeView === "projects" ? <ProjectsView /> : activeView === "approvals" ? <WorkflowView /> : activeView === "finance" ? <FinanceView /> : activeView === "hr" ? <HRView /> : activeView === "sales" ? <SalesView /> : activeView === "it" ? <ITView /> : activeView === "reports" ? <ReportsView /> : activeView === "calendar" ? <CalendarView /> : activeView === "attendance" ? <AttendanceView /> : activeView === "leave" ? <LeaveView /> : activeView === "schedule" ? <ScheduleView /> : activeView === "activity" ? <ActivityView /> : activeView === "settings" ? <SettingsView /> : <section className="content-wrap">
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
  const [conflicts, setConflicts] = useState<Array<{ id: string; entity: string; action: string; reason: string; client_payload: Record<string, unknown> }>>([]);
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

  const deleteTask = async (taskId: string) => {
    if (!window.confirm("Delete this task for everyone?")) return;
    try {
      const response = await fetch(`${apiBase}/tasks/${taskId}`, { method: "DELETE", credentials: "include" });
      if (!response.ok) throw new Error("failed");
      setTasks((current) => current.filter((task) => task.id !== taskId));
      await deleteLocalTask(taskId);
      setMessage("Task deleted");
    } catch { setMessage("Could not delete task"); }
  };

  const loadConflicts = async () => {
    try { const response = await fetch(`${apiBase}/sync/conflicts`, { credentials: "include" }); if (response.ok) setConflicts(await response.json()); } catch { /* offline */ }
  };
  useEffect(() => { void loadConflicts(); }, []);
  const resolveConflict = async (id: string, resolution: "accept_client" | "accept_server") => {
    const response = await fetch(`${apiBase}/sync/conflicts/resolve`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ id, resolution }) });
    setMessage(response.ok ? "Conflict resolved" : "Could not resolve conflict");
    await loadConflicts();
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
    {conflicts.length > 0 && <div className="conflict-panel">
      <div className="section-heading"><div><p className="eyebrow">Sync conflicts</p><h2>{conflicts.length} to review</h2></div></div>
      <div className="record-list">{conflicts.map((conflict) => <div className="record-row" key={conflict.id}><span className="record-avatar leave-rejected">!</span><span className="record-copy"><strong>{String(conflict.client_payload.title ?? conflict.entity)}</strong><small>{conflict.entity} · {conflict.reason}</small></span><span className="record-actions"><button className="text-button" type="button" onClick={() => void resolveConflict(conflict.id, "accept_client")}>Keep mine</button><button className="text-button muted" type="button" onClick={() => void resolveConflict(conflict.id, "accept_server")}>Keep server</button></span></div>)}</div>
    </div>}
    <form className="task-composer" onSubmit={createTask}><input value={title} onChange={(event) => setTitle(event.target.value)} placeholder="What needs to get done?" required /><Dropdown value={priority} options={PRIORITY_OPTIONS} onChange={setPriority} ariaLabel="Priority" /><DatePicker value={dueAt} onChange={setDueAt} placeholder="Due date" ariaLabel="Due date" /><Dropdown value={assignedTo} options={[{ value: "", label: "Assign to me" }, ...users.map((user) => ({ value: user.id, label: user.display_name }))]} onChange={setAssignedTo} ariaLabel="Assignee" /><Dropdown value={recurrenceRule} options={RECURRENCE_OPTIONS} onChange={setRecurrenceRule} ariaLabel="Repeat" /><button className="primary-button" type="submit" disabled={saving}>{saving ? "Saving…" : "Add task"}</button></form>
    <div className="section-heading task-heading"><div><p className="eyebrow">Queue</p><h2>{tasks.length} tasks</h2></div><div className="select-inline"><Dropdown value={taskScope} options={TASK_SCOPE_OPTIONS} onChange={setTaskScope} ariaLabel="Task scope" /></div></div>
    <div className="task-list">{tasks.map((task) => { const syncStatus = outboxStatuses.get(task.id); return <div key={task.id}><div className="task-row"><span className={`task-status ${task.status}`} /><span className="record-copy"><strong>{task.title}</strong><small>{task.priority} priority · {task.dueAt ? `due ${task.dueAt.slice(0, 10)} · ` : ""}{task.assignedTo ? "assigned" : "unassigned"} · {syncStatus === "conflict" ? "server conflict" : syncStatus === "failed" ? "sync failed" : syncStatus === "pending" ? "waiting to sync" : "synced"}</small></span><button className="text-button" type="button" onClick={() => void openComments(task.id)}>Comments</button>{!syncStatus && <button className="text-button muted" type="button" onClick={() => void deleteTask(task.id)}>Delete</button>}{syncStatus === "failed" || syncStatus === "conflict" ? <span className="task-actions"><button className="text-button" type="button" onClick={() => void retryTask(task.id)}>Retry</button>{syncStatus === "conflict" && <button className="text-button muted" type="button" onClick={() => void discardTask(task.id)}>Discard</button>}</span> : <span className={`status-pill priority-${task.priority}`}>{task.status.replace("_", " ")}</span>}</div>{commentTaskId === task.id && <div className="comment-thread">{comments.map((comment) => <p key={comment.id}><strong>{comment.author_name}</strong> {comment.body}</p>)}<form onSubmit={addComment}><input value={commentBody} onChange={(event) => setCommentBody(event.target.value)} placeholder="Write a comment or @mention someone" /><button className="text-button" type="submit">Post</button></form></div>}</div>; })}</div>
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

  return <section className="content-wrap"><div className="page-heading"><div><p className="eyebrow">Attendance</p><h1>Be present, clearly.</h1><p className="lede">Check-in and check-out remain available when the company server is offline.</p></div></div>{message && <p className="inline-message">{message}</p>}<div className="attendance-today"><div><p className="eyebrow">Today · {today}</p><h2>{todayRecord?.check_in_at ? todayRecord.check_out_at ? "Workday complete" : "Currently working" : "Not checked in"}</h2></div><div className="attendance-actions"><button className="primary-button" type="button" onClick={() => void mark("check_in")} disabled={Boolean(todayRecord?.check_in_at)}>Check in</button><button className="text-button" type="button" onClick={() => void mark("check_out")} disabled={!todayRecord?.check_in_at || Boolean(todayRecord.check_out_at)}>Check out</button></div></div><div className="section-heading task-heading"><div><p className="eyebrow">History</p><h2>Recent attendance</h2></div></div><div className="record-list">{records.map((record) => <div className="record-row" key={record.id}><span className="record-avatar"><Clock3 size={15} /></span><span className="record-copy"><strong>{record.work_date}</strong><small>{record.check_in_at ? new Date(record.check_in_at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }) : "—"} → {record.check_out_at ? new Date(record.check_out_at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }) : "—"}</small></span><span className="status-pill">{record.status}</span></div>)}</div>{managerRecords.length > 0 && <><div className="section-heading task-heading"><div><p className="eyebrow">Manager view</p><h2>Team attendance</h2></div></div><div className="record-list">{managerRecords.map((record) => <div className="record-row" key={record.id}><span className="record-avatar"><UsersRound size={15} /></span><span className="record-copy"><strong>{record.display_name}</strong><small>{record.work_date} · {record.check_in_at ? new Date(record.check_in_at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }) : "—"} → {record.check_out_at ? new Date(record.check_out_at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }) : "—"}</small></span><span className="status-pill">{record.status}</span></div>)}</div><form className="compact-form attendance-correction" onSubmit={submitCorrection}><p className="eyebrow">Correction</p><Dropdown value={correction.user_id} options={[{ value: "", label: "Select person" }, ...users.map((user) => ({ value: user.id, label: user.display_name }))]} onChange={(value) => setCorrection({ ...correction, user_id: value })} ariaLabel="Person" placeholder="Select person" /><DatePicker value={correction.work_date} onChange={(value) => setCorrection({ ...correction, work_date: value })} ariaLabel="Work date" /><div className="form-grid"><TimePicker value={correction.check_in_at} onChange={(value) => setCorrection({ ...correction, check_in_at: value })} ariaLabel="Check in" /><TimePicker value={correction.check_out_at} onChange={(value) => setCorrection({ ...correction, check_out_at: value })} ariaLabel="Check out" /><input placeholder="Reason or note" value={correction.note} onChange={(event) => setCorrection({ ...correction, note: event.target.value })} /></div><button className="primary-button" type="submit">Save correction</button></form></>}</section>;
}

function LeaveView() {
  const [requests, setRequests] = useState<Array<LeaveRequest & LocalLeave>>([]);
  const [form, setForm] = useState({ leave_type: "annual", start_date: new Date().toISOString().slice(0, 10), end_date: new Date().toISOString().slice(0, 10), reason: "" });
  const [message, setMessage] = useState("");
  const leaveTypes = useLookup("leave_type");
  const labelFor = (value: string) => leaveTypes.find((option) => option.value === value)?.label ?? value;
  const colorFor = (value: string) => leaveTypes.find((option) => option.value === value)?.color;
  const load = async () => { try { await syncPendingOperations(apiBase); const response = await fetch(`${apiBase}/leave`, { credentials: "include" }); if (!response.ok) throw new Error("offline"); const remote = await response.json() as Array<LeaveRequest & LocalLeave>; setRequests(remote); await cacheLeave(remote); } catch { try { setRequests(await getLocalLeave() as Array<LeaveRequest & LocalLeave>); } catch { setRequests([]); } } };
  useEffect(() => { void load(); const onOnline = () => void load(); window.addEventListener("online", onOnline); return () => window.removeEventListener("online", onOnline); }, []);
  const createRequest = async (event: FormEvent<HTMLFormElement>) => { event.preventDefault(); const now = new Date().toISOString(); const local: LocalLeave = { id: crypto.randomUUID(), requested_by: "local", leave_type: form.leave_type, start_date: form.start_date, end_date: form.end_date, reason: form.reason, status: "pending" }; try { const response = await fetch(`${apiBase}/leave`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify(form) }); if (!response.ok) throw new Error("offline"); const saved = await response.json() as LeaveRequest & LocalLeave; setRequests((current) => [saved, ...current]); await cacheLeave([saved]); setMessage("Leave request submitted"); } catch { await queueOperation({ id: `leave:${local.id}`, entity: "leave", action: "create", payload: local, createdAt: now }); await cacheLeave([local]); setRequests((current) => [local, ...current]); setMessage("Saved offline · will sync when the server is reachable"); } setForm((current) => ({ ...current, reason: "" })); };
  const decide = async (id: string, action: "approve" | "reject") => { const response = await fetch(`${apiBase}/leave/${action}`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ id }) }); setMessage(response.ok ? `Request ${action}d` : "You do not have permission to decide this request"); if (response.ok) void load(); };
  return <section className="content-wrap"><div className="page-heading"><div><p className="eyebrow">Leave</p><h1>Plan time away.</h1><p className="lede">Requests stay visible to the team and move through a clear approval path.</p></div></div>{message && <p className="inline-message">{message}</p>}<form className="leave-form" onSubmit={createRequest}><p className="eyebrow">New request</p><div className="form-grid"><Dropdown value={form.leave_type} options={leaveTypes} onChange={(value) => setForm({ ...form, leave_type: value })} ariaLabel="Leave type" placeholder="Leave type" /><DatePicker value={form.start_date} onChange={(value) => setForm({ ...form, start_date: value })} ariaLabel="Start date" /><DatePicker value={form.end_date} onChange={(value) => setForm({ ...form, end_date: value })} ariaLabel="End date" /></div><input placeholder="Reason (optional)" value={form.reason} onChange={(event) => setForm({ ...form, reason: event.target.value })} /><button className="primary-button" type="submit">Submit request</button></form><div className="section-heading task-heading"><div><p className="eyebrow">Requests</p><h2>{requests.length} requests</h2></div></div><div className="record-list">{requests.map((request) => <div className="record-row" key={request.id}><span className="record-avatar"><CalendarDays size={15} /></span><span className="record-copy"><strong>{request.display_name ?? "My request"} · <span className="leave-type-tag">{colorFor(request.leave_type) && <span className="dropdown-swatch" style={{ background: colorFor(request.leave_type) }} />}{labelFor(request.leave_type)}</span></strong><small>{request.start_date} → {request.end_date}{request.reason ? ` · ${request.reason}` : ""}</small></span><span className={`status-pill leave-${request.status}`}>{request.status}</span>{request.status === "pending" && <span className="task-actions"><button className="text-button" type="button" onClick={() => void decide(request.id, "approve")}>Approve</button><button className="text-button muted" type="button" onClick={() => void decide(request.id, "reject")}>Reject</button></span>}</div>)}</div></section>;
}

function ScheduleView() {
  const [shifts, setShifts] = useState<LocalShift[]>([]);
  const [form, setForm] = useState({ title: "", shift_date: new Date().toISOString().slice(0, 10), starts_at: "09:00", ends_at: "17:00", note: "" });
  const [message, setMessage] = useState("");
  const load = async () => { try { await syncPendingOperations(apiBase); const response = await fetch(`${apiBase}/shifts`, { credentials: "include" }); if (!response.ok) throw new Error("offline"); const remote = await response.json() as LocalShift[]; setShifts(remote); await cacheShifts(remote); } catch { try { setShifts(await getLocalShifts()); } catch { setShifts([]); } } };
  useEffect(() => { void load(); const onOnline = () => void load(); window.addEventListener("online", onOnline); return () => window.removeEventListener("online", onOnline); }, []);
  const createShift = async (event: FormEvent<HTMLFormElement>) => { event.preventDefault(); const now = new Date().toISOString(); const local: LocalShift = { id: crypto.randomUUID(), assigned_to: "local", ...form, status: "scheduled" }; try { const response = await fetch(`${apiBase}/shifts`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify(form) }); if (!response.ok) throw new Error("offline"); const saved = await response.json() as LocalShift; setShifts((current) => [...current, saved]); await cacheShifts([saved]); setMessage("Shift scheduled"); } catch { await queueOperation({ id: `shift:${local.id}`, entity: "shift", action: "create", payload: local, createdAt: now }); await cacheShifts([local]); setShifts((current) => [...current, local]); setMessage("Saved offline · will sync when the server is reachable"); } setForm((current) => ({ ...current, title: "", note: "" })); };
  return <section className="content-wrap"><div className="page-heading"><div><p className="eyebrow">Schedule</p><h1>Make time visible.</h1><p className="lede">Shifts stay available on this device and sync back to the company schedule.</p></div></div>{message && <p className="inline-message">{message}</p>}<form className="leave-form" onSubmit={createShift}><p className="eyebrow">New shift</p><div className="form-grid"><input placeholder="Shift title" value={form.title} onChange={(event) => setForm({ ...form, title: event.target.value })} required /><DatePicker value={form.shift_date} onChange={(value) => setForm({ ...form, shift_date: value })} ariaLabel="Shift date" /><TimePicker value={form.starts_at} onChange={(value) => setForm({ ...form, starts_at: value })} ariaLabel="Start time" /></div><div className="form-grid"><TimePicker value={form.ends_at} onChange={(value) => setForm({ ...form, ends_at: value })} ariaLabel="End time" /><input placeholder="Note (optional)" value={form.note} onChange={(event) => setForm({ ...form, note: event.target.value })} /><button className="primary-button" type="submit">Schedule shift</button></div></form><div className="section-heading task-heading"><div><p className="eyebrow">Upcoming</p><h2>{shifts.length} shifts</h2></div></div><div className="record-list">{shifts.map((shift) => <div className="record-row" key={shift.id}><span className="record-avatar"><CalendarDays size={15} /></span><span className="record-copy"><strong>{shift.title}</strong><small>{shift.shift_date} · {shift.starts_at.slice(0, 5)} → {shift.ends_at.slice(0, 5)}</small></span><span className="status-pill">{shift.status}</span></div>)}</div></section>;
}

function ActivityView() {
  const [entries, setEntries] = useState<AuditEntry[]>([]);
  useEffect(() => { void fetch(`${apiBase}/audit-logs`, { credentials: "include" }).then((response) => response.ok ? response.json() : []).then(setEntries).catch(() => setEntries([])); }, []);
  return <section className="content-wrap"><div className="page-heading"><div><p className="eyebrow">Activity</p><h1>Know what changed.</h1><p className="lede">A durable record of privileged actions in this private installation.</p></div></div><div className="section-heading task-heading"><div><p className="eyebrow">Audit trail</p><h2>{entries.length} events</h2></div></div><div className="activity-list">{entries.map((entry) => <div className="activity-row" key={entry.id}><span className="activity-icon"><Activity size={16} /></span><span className="activity-copy"><strong>{entry.action}</strong><small>{entry.actor_name} · {entry.entity_type} · {new Date(entry.created_at).toLocaleString()}</small></span></div>)}</div></section>;
}

type DashboardSummary = { scope: string; open_tasks: number; in_progress_tasks: number; overdue_tasks: number; done_tasks: number; approvals_waiting: number; upcoming_events: number; unread_notifications: number; department_breakdown: Array<{ department: string; open_tasks: number }> };
type SpendingSummary = { period: string; from: string; to: string; total_spent: number; expense_count: number; paid_count: number; by_category: Array<{ category: string; amount: number; count: number }> };
type AttendanceSummary = { period: string; from: string; to: string; present_days: number; remote_days: number; leave_days: number; absent_days: number; checked_in_days: number; completed_days: number; total_hours: number; average_hours: number };
type SalesSummary = { period: string; from: string; to: string; module_available: boolean; message: string; lead_count: number; opportunity_count: number; pipeline_value: number; won_value: number };
type ReportDefinition = { id: string; code: string; name: string; report_type: string; description: string; config: Record<string, unknown>; created_at: string };
type ReportSchedule = { id: string; definition_id: string; definition_name: string; report_type: string; cadence: string; next_run_at: string; last_run_at: string | null; active: boolean; created_at: string };

function DashboardOverview() {
  const [scope, setScope] = useState("organization");
  const [summary, setSummary] = useState<DashboardSummary | null>(null);
  const [spending, setSpending] = useState<SpendingSummary | null>(null);
  const [attendance, setAttendance] = useState<AttendanceSummary | null>(null);

  useEffect(() => {
    let active = true;
    void (async () => {
      const response = await fetch(`${apiBase}/dashboard/summary?scope=${scope}`, { credentials: "include" });
      if (response.ok && active) setSummary(await response.json());
    })();
    return () => { active = false; };
  }, [scope]);

  useEffect(() => {
    let active = true;
    void (async () => {
      const [spendRes, attendRes] = await Promise.all([
        fetch(`${apiBase}/analytics/spending?period=30d`, { credentials: "include" }),
        fetch(`${apiBase}/analytics/attendance?period=30d`, { credentials: "include" }),
      ]);
      if (active && spendRes.ok) setSpending(await spendRes.json());
      if (active && attendRes.ok) setAttendance(await attendRes.json());
    })();
    return () => { active = false; };
  }, []);

  const showBreakdown = (scope === "organization" || scope === "department") && (summary?.department_breakdown?.length ?? 0) > 0;

  return <>
    <div className="section-heading" style={{ marginTop: 34 }}>
      <div><p className="eyebrow">Dashboard</p><h2>Where things stand.</h2></div>
      <div className="select-inline"><Dropdown value={scope} options={DASH_SCOPE_OPTIONS} onChange={setScope} ariaLabel="Dashboard scope" /></div>
    </div>
    <div className="metric-grid">
      <Metric label="Open work" value={String(summary?.open_tasks ?? 0)} change={`${summary?.in_progress_tasks ?? 0} in progress`} detail="tasks not yet done" />
      <Metric label="Overdue" value={String(summary?.overdue_tasks ?? 0)} change={summary && summary.overdue_tasks > 0 ? "needs attention" : "on track"} detail="past their due date" warning={Boolean(summary && summary.overdue_tasks > 0)} />
      <Metric label="Approvals waiting" value={String(summary?.approvals_waiting ?? 0)} change={`${summary?.upcoming_events ?? 0} events soon`} detail="need your decision" warning={Boolean(summary && summary.approvals_waiting > 0)} />
    </div>
    {(spending || attendance) && <div className="metric-grid">
      <Metric label="Spending (30d)" value={spending ? `$${spending.total_spent.toFixed(0)}` : "—"} change={`${spending?.expense_count ?? 0} expenses`} detail={`${spending?.paid_count ?? 0} paid`} />
      <Metric label="Attendance (30d)" value={String(attendance?.present_days ?? 0)} change={`${attendance?.total_hours ?? 0}h logged`} detail={`${attendance?.completed_days ?? 0} completed days`} />
      <Metric label="Avg hours / day" value={String(attendance?.average_hours ?? 0)} change={`${attendance?.remote_days ?? 0} remote`} detail="completed check-outs" />
    </div>}
    {showBreakdown && <>
      <div className="section-heading"><div><p className="eyebrow">By department</p><h2>Open work across the company.</h2></div></div>
      <div className="record-list">{summary!.department_breakdown.map((row) => <div className="record-row" key={row.department}><span className="record-avatar department-avatar"><BriefcaseBusiness size={15} /></span><span className="record-copy"><strong>{row.department}</strong><small>{row.open_tasks} open {row.open_tasks === 1 ? "task" : "tasks"}</small></span><span className="status-pill">{row.open_tasks}</span></div>)}</div>
    </>}
  </>;
}

function ReportsView() {
  const [period, setPeriod] = useState("30d");
  const [spending, setSpending] = useState<SpendingSummary | null>(null);
  const [attendance, setAttendance] = useState<AttendanceSummary | null>(null);
  const [sales, setSales] = useState<SalesSummary | null>(null);
  const [definitions, setDefinitions] = useState<ReportDefinition[]>([]);
  const [schedules, setSchedules] = useState<ReportSchedule[]>([]);
  const [exportPreview, setExportPreview] = useState("");
  const [message, setMessage] = useState("");
  const [reportForm, setReportForm] = useState({ code: "", name: "", report_type: "spending", description: "", period: "30d" });
  const [scheduleForm, setScheduleForm] = useState({ definition_id: "", cadence: "weekly" });
  const [showCreate, setShowCreate] = useState(false);

  const load = async () => {
    const [spendRes, attendRes, salesRes, defsRes, schedRes] = await Promise.all([
      fetch(`${apiBase}/analytics/spending?period=${period}`, { credentials: "include" }),
      fetch(`${apiBase}/analytics/attendance?period=${period}`, { credentials: "include" }),
      fetch(`${apiBase}/analytics/sales?period=${period}`, { credentials: "include" }),
      fetch(`${apiBase}/reports`, { credentials: "include" }),
      fetch(`${apiBase}/reports/schedules`, { credentials: "include" }),
    ]);
    if (spendRes.ok) setSpending(await spendRes.json());
    if (attendRes.ok) setAttendance(await attendRes.json());
    if (salesRes.ok) setSales(await salesRes.json());
    if (defsRes.ok) setDefinitions(await defsRes.json());
    if (schedRes.ok) setSchedules(await schedRes.json());
  };

  useEffect(() => { void load(); }, [period]);

  const createReport = async (event: FormEvent) => {
    event.preventDefault();
    const response = await fetch(`${apiBase}/reports`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        code: reportForm.code,
        name: reportForm.name,
        report_type: reportForm.report_type,
        description: reportForm.description,
        config: { period: reportForm.period },
      }),
    });
    setMessage(response.ok ? "Report saved." : "Could not save report.");
    if (response.ok) {
      setReportForm({ code: "", name: "", report_type: "spending", description: "", period: "30d" });
      setShowCreate(false);
      await load();
    }
  };

  const createSchedule = async (event: FormEvent) => {
    event.preventDefault();
    const response = await fetch(`${apiBase}/reports/schedules`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(scheduleForm),
    });
    setMessage(response.ok ? "Schedule created." : "Could not create schedule.");
    if (response.ok) {
      setScheduleForm({ definition_id: "", cadence: "weekly" });
      await load();
    }
  };

  const exportReport = async (id: string) => {
    const response = await fetch(`${apiBase}/reports/${id}/export`, { method: "POST", credentials: "include" });
    if (!response.ok) {
      setMessage("Export failed.");
      return;
    }
    const payload = await response.json();
    setExportPreview(JSON.stringify(payload, null, 2));
    setMessage("Export ready — recorded in the audit trail.");
  };

  return <section className="content-wrap">
    <div className="page-heading">
      <div>
        <p className="eyebrow">Reports</p>
        <h1>Summaries &amp; exports.</h1>
        <p className="lede">Spending, attendance, and sales summaries with saved reports, schedules, and an export audit trail.</p>
      </div>
      <div style={{ display: "flex", gap: 12, alignItems: "center" }}>
        <div className="select-inline"><Dropdown value={period} options={PERIOD_OPTIONS} onChange={setPeriod} ariaLabel="Summary period" /></div>
        <button className="primary-button" type="button" onClick={() => setShowCreate((value) => !value)}>{showCreate ? "Close" : "Save report"}</button>
      </div>
    </div>
    {message && <p className="form-message">{message}</p>}

    <div className="metric-grid">
      <Metric label="Total spent" value={spending ? `$${spending.total_spent.toFixed(2)}` : "—"} change={`${spending?.expense_count ?? 0} expenses`} detail={`${spending?.from ?? ""} → ${spending?.to ?? ""}`} />
      <Metric label="Present days" value={String(attendance?.present_days ?? 0)} change={`${attendance?.leave_days ?? 0} leave · ${attendance?.absent_days ?? 0} absent`} detail={`${attendance?.total_hours ?? 0} hours`} />
      <Metric label="Sales pipeline" value={sales?.module_available ? `$${(sales.pipeline_value ?? 0).toFixed(0)}` : "—"} change={sales?.module_available ? `${sales.opportunity_count} opportunities` : "CRM not installed"} detail={sales?.message ?? "Reserved summary shape"} />
    </div>

    {(spending?.by_category?.length ?? 0) > 0 && <>
      <div className="section-heading task-heading"><div><p className="eyebrow">By category</p><h2>Spending breakdown</h2></div></div>
      <div className="record-list">{spending!.by_category.map((row) => <div className="record-row" key={row.category}><span className="record-avatar"><Wallet size={15} /></span><span className="record-copy"><strong>{row.category}</strong><small>{row.count} expense{row.count === 1 ? "" : "s"}</small></span><span className="status-pill">${row.amount.toFixed(2)}</span></div>)}</div>
    </>}

    {showCreate && <form className="inline-form" onSubmit={(event) => void createReport(event)}>
      <input required placeholder="Code" value={reportForm.code} onChange={(event) => setReportForm({ ...reportForm, code: event.target.value })} />
      <input required placeholder="Name" value={reportForm.name} onChange={(event) => setReportForm({ ...reportForm, name: event.target.value })} />
      <Dropdown value={reportForm.report_type} options={REPORT_TYPE_OPTIONS} onChange={(value) => setReportForm({ ...reportForm, report_type: value })} ariaLabel="Report type" />
      <Dropdown value={reportForm.period} options={PERIOD_SHORT_OPTIONS} onChange={(value) => setReportForm({ ...reportForm, period: value })} ariaLabel="Report period" />
      <input placeholder="Description" value={reportForm.description} onChange={(event) => setReportForm({ ...reportForm, description: event.target.value })} />
      <button className="primary-button" type="submit">Save</button>
    </form>}

    <div className="section-heading task-heading"><div><p className="eyebrow">Saved reports</p><h2>{definitions.length}</h2></div></div>
    <div className="record-list">{definitions.length === 0 ? <p className="lede">No saved reports yet.</p> : definitions.map((report) => <div className="record-row" key={report.id}><span className="record-avatar"><LineChart size={15} /></span><span className="record-copy"><strong>{report.name}</strong><small>{report.report_type} · {report.code}{report.description ? ` · ${report.description}` : ""}</small></span><span className="record-actions"><button className="text-button" type="button" onClick={() => void exportReport(report.id)}>Export</button></span></div>)}</div>

    <div className="section-heading task-heading"><div><p className="eyebrow">Schedules</p><h2>{schedules.length}</h2></div></div>
    <form className="inline-form" onSubmit={(event) => void createSchedule(event)}>
      <Dropdown value={scheduleForm.definition_id} options={[{ value: "", label: "Select report" }, ...definitions.map((report) => ({ value: report.id, label: report.name }))]} onChange={(value) => setScheduleForm({ ...scheduleForm, definition_id: value })} ariaLabel="Report" placeholder="Select report" />
      <Dropdown value={scheduleForm.cadence} options={CADENCE_OPTIONS} onChange={(value) => setScheduleForm({ ...scheduleForm, cadence: value })} ariaLabel="Cadence" />
      <button className="primary-button" type="submit">Schedule</button>
    </form>
    <div className="record-list">{schedules.map((schedule) => <div className="record-row" key={schedule.id}><span className="record-avatar"><Clock3 size={15} /></span><span className="record-copy"><strong>{schedule.definition_name}</strong><small>{schedule.cadence} · next {new Date(schedule.next_run_at).toLocaleString()}{schedule.last_run_at ? ` · last ${new Date(schedule.last_run_at).toLocaleString()}` : ""}</small></span><span className={`status-pill ${schedule.active ? "leave-approved" : ""}`}>{schedule.active ? "active" : "paused"}</span></div>)}</div>

    {exportPreview && <>
      <div className="section-heading task-heading"><div><p className="eyebrow">Last export</p><h2>JSON snapshot</h2></div></div>
      <pre className="code-block" style={{ whiteSpace: "pre-wrap", fontSize: 12, maxHeight: 320, overflow: "auto" }}>{exportPreview}</pre>
    </>}
  </section>;
}

const expenseStatusClass: Record<string, string> = { approved: "leave-approved", rejected: "leave-rejected", in_review: "leave-pending", paid: "leave-approved", submitted: "leave-pending", draft: "" };

function FinanceView() {
  const [vendors, setVendors] = useState<VendorItem[]>([]);
  const [expenses, setExpenses] = useState<ExpenseItem[]>([]);
  const [expenseForm, setExpenseForm] = useState({ description: "", amount: "", category: "general", vendor_id: "" });
  const [vendorForm, setVendorForm] = useState({ name: "", contact_email: "", contact_phone: "" });
  const [showVendorForm, setShowVendorForm] = useState(false);
  const [message, setMessage] = useState("");

  const load = async () => {
    const [vendorsResponse, expensesResponse] = await Promise.all([
      fetch(`${apiBase}/vendors`, { credentials: "include" }),
      fetch(`${apiBase}/expenses`, { credentials: "include" }),
    ]);
    if (vendorsResponse.ok) setVendors(await vendorsResponse.json());
    if (expensesResponse.ok) setExpenses(await expensesResponse.json());
  };
  useEffect(() => { void load(); }, []);

  const createExpense = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const response = await fetch(`${apiBase}/expenses`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ description: expenseForm.description, amount: Number(expenseForm.amount), category: expenseForm.category, vendor_id: expenseForm.vendor_id }) });
    setMessage(response.ok ? "Expense drafted" : ((await response.json()).error?.message ?? "Could not create expense"));
    if (response.ok) { setExpenseForm({ description: "", amount: "", category: "general", vendor_id: "" }); await load(); }
  };
  const createVendor = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const response = await fetch(`${apiBase}/vendors`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify(vendorForm) });
    setMessage(response.ok ? "Vendor added" : ((await response.json()).error?.message ?? "Could not add vendor"));
    if (response.ok) { setVendorForm({ name: "", contact_email: "", contact_phone: "" }); setShowVendorForm(false); await load(); }
  };
  const act = async (expenseId: string, action: "submit" | "pay") => {
    const response = await fetch(`${apiBase}/expenses/${expenseId}/${action}`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify({}) });
    setMessage(response.ok ? (action === "submit" ? "Expense submitted for approval" : "Expense marked paid") : ((await response.json()).error?.message ?? "Action failed"));
    await load();
  };

  const money = (item: ExpenseItem) => `${item.currency} ${item.amount.toLocaleString(undefined, { minimumFractionDigits: 2 })}`;

  return <section className="content-wrap">
    <div className="page-heading"><div><p className="eyebrow">Finance</p><h1>Expenses & vendors.</h1><p className="lede">Submit expenses for approval through the company workflow, then mark them paid.</p></div><button className="primary-button" type="button" onClick={() => setShowVendorForm((value) => !value)}>{showVendorForm ? "Close" : "Add vendor"}</button></div>
    {message && <p className="inline-message">{message}</p>}

    <form className="task-composer workflow-composer" onSubmit={createExpense}>
      <input placeholder="What was the expense for?" value={expenseForm.description} onChange={(event) => setExpenseForm({ ...expenseForm, description: event.target.value })} required />
      <input placeholder="Amount" inputMode="decimal" value={expenseForm.amount} onChange={(event) => setExpenseForm({ ...expenseForm, amount: event.target.value.replace(/[^0-9.]/g, "") })} required />
      <Dropdown value={expenseForm.vendor_id} options={[{ value: "", label: "No vendor" }, ...vendors.map((vendor) => ({ value: vendor.id, label: vendor.name }))]} onChange={(value) => setExpenseForm({ ...expenseForm, vendor_id: value })} ariaLabel="Vendor" placeholder="No vendor" />
      <button className="primary-button" type="submit">Draft expense</button>
    </form>

    {showVendorForm && <form className="user-form" onSubmit={createVendor}><p className="eyebrow">New vendor</p><div className="form-grid"><input placeholder="Vendor name" value={vendorForm.name} onChange={(event) => setVendorForm({ ...vendorForm, name: event.target.value })} required /><input placeholder="Contact email" value={vendorForm.contact_email} onChange={(event) => setVendorForm({ ...vendorForm, contact_email: event.target.value })} /><input placeholder="Contact phone" value={vendorForm.contact_phone} onChange={(event) => setVendorForm({ ...vendorForm, contact_phone: event.target.value })} /></div><button className="primary-button" type="submit">Add vendor</button></form>}

    <div className="section-heading task-heading"><div><p className="eyebrow">Expenses</p><h2>{expenses.length} records</h2></div></div>
    <div className="record-list">{expenses.map((expense) => <div className="record-row" key={expense.id}><span className="record-avatar"><Wallet size={15} /></span><span className="record-copy"><strong>{expense.description} · {money(expense)}</strong><small>{expense.category}{expense.vendor_name ? ` · ${expense.vendor_name}` : ""} · by {expense.submitter_name}{expense.approval_status ? ` · approval: ${expense.approval_status.replace("_", " ")}` : ""}</small></span><span className={`status-pill ${expenseStatusClass[expense.approval_status || expense.status] ?? ""}`}>{expense.status}</span>{expense.status === "draft" && <span className="record-actions"><button className="text-button" type="button" onClick={() => void act(expense.id, "submit")}>Submit</button></span>}{expense.status === "submitted" && (expense.approval_status === "approved" || expense.approval_status === "") && <span className="record-actions"><button className="text-button" type="button" onClick={() => void act(expense.id, "pay")}>Mark paid</button></span>}</div>)}</div>

    {vendors.length > 0 && <><div className="section-heading task-heading"><div><p className="eyebrow">Vendors</p><h2>{vendors.length}</h2></div></div><div className="record-list">{vendors.map((vendor) => <div className="record-row" key={vendor.id}><span className="record-avatar department-avatar">{vendor.name.slice(0, 2).toUpperCase()}</span><span className="record-copy"><strong>{vendor.name}</strong><small>{vendor.contact_email || "no email"}{vendor.contact_phone ? ` · ${vendor.contact_phone}` : ""}</small></span><span className="status-pill">{vendor.status}</span></div>)}</div></>}
    <FinanceExtras />
  </section>;
}

function HRView() {
  const [profiles, setProfiles] = useState<Array<{ user_id: string; display_name: string; email: string; phone: string; hire_date: string; employment_type: string; job_title: string }>>([]);
  const [onboarding, setOnboarding] = useState<Array<{ id: string; user_id: string; user_name: string; title: string; status: string }>>([]);
  const [documents, setDocuments] = useState<Array<{ id: string; user_name: string; title: string; doc_type: string }>>([]);
  const [profileForm, setProfileForm] = useState({ user_id: "", phone: "", hire_date: "", employment_type: "full_time" });
  const [onboardForm, setOnboardForm] = useState({ user_id: "", title: "" });
  const [docForm, setDocForm] = useState({ user_id: "", title: "", doc_type: "contract" });
  const [message, setMessage] = useState("");

  const load = async () => {
    const [p, o, d] = await Promise.all([
      fetch(`${apiBase}/hr/profiles`, { credentials: "include" }),
      fetch(`${apiBase}/hr/onboarding`, { credentials: "include" }),
      fetch(`${apiBase}/hr/documents`, { credentials: "include" }),
    ]);
    if (p.ok) setProfiles(await p.json());
    if (o.ok) setOnboarding(await o.json());
    if (d.ok) setDocuments(await d.json());
  };
  useEffect(() => { void load(); }, []);
  const post = async (url: string, body: unknown, ok: string) => { const r = await fetch(`${apiBase}${url}`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) }); setMessage(r.ok ? ok : ((await r.json()).error?.message ?? "Action failed")); await load(); return r.ok; };

  return <section className="content-wrap">
    <div className="page-heading"><div><p className="eyebrow">Human Resources</p><h1>People operations.</h1><p className="lede">Employee profiles, onboarding, and HR documents.</p></div></div>
    {message && <p className="inline-message">{message}</p>}
    <div className="section-heading task-heading"><div><p className="eyebrow">Directory</p><h2>{profiles.length} employees</h2></div></div>
    <div className="record-list">{profiles.map((p) => <div className="record-row" key={p.user_id}><span className="record-avatar">{p.display_name.slice(0, 1).toUpperCase()}</span><span className="record-copy"><strong>{p.display_name}{p.job_title ? ` · ${p.job_title}` : ""}</strong><small>{p.email}{p.phone ? ` · ${p.phone}` : ""}{p.hire_date ? ` · hired ${p.hire_date}` : ""} · {p.employment_type.replace("_", " ")}</small></span></div>)}</div>
    <form className="user-form" onSubmit={async (e) => { e.preventDefault(); if (await post("/hr/profiles", profileForm, "Profile saved")) setProfileForm({ user_id: "", phone: "", hire_date: "", employment_type: "full_time" }); }}><p className="eyebrow">Update employee profile</p><div className="form-grid"><Dropdown value={profileForm.user_id} options={[{ value: "", label: "Select person" }, ...profiles.map((p) => ({ value: p.user_id, label: p.display_name }))]} onChange={(value) => setProfileForm({ ...profileForm, user_id: value })} ariaLabel="Person" placeholder="Select person" /><input placeholder="Phone" value={profileForm.phone} onChange={(e) => setProfileForm({ ...profileForm, phone: e.target.value })} /><DatePicker value={profileForm.hire_date} onChange={(value) => setProfileForm({ ...profileForm, hire_date: value })} placeholder="Hire date" ariaLabel="Hire date" /></div><div className="select-inline"><Dropdown value={profileForm.employment_type} options={EMPLOYMENT_OPTIONS} onChange={(value) => setProfileForm({ ...profileForm, employment_type: value })} ariaLabel="Employment type" /></div><button className="primary-button" type="submit">Save profile</button></form>

    <div className="section-heading task-heading"><div><p className="eyebrow">Onboarding</p><h2>{onboarding.filter((o) => o.status !== "done").length} open</h2></div></div>
    <form className="task-composer workflow-composer" onSubmit={async (e) => { e.preventDefault(); if (await post("/hr/onboarding", onboardForm, "Onboarding task added")) setOnboardForm({ user_id: "", title: "" }); }}><Dropdown value={onboardForm.user_id} options={[{ value: "", label: "New hire" }, ...profiles.map((p) => ({ value: p.user_id, label: p.display_name }))]} onChange={(value) => setOnboardForm({ ...onboardForm, user_id: value })} ariaLabel="New hire" placeholder="New hire" /><input placeholder="Onboarding task" value={onboardForm.title} onChange={(e) => setOnboardForm({ ...onboardForm, title: e.target.value })} required /><button className="primary-button" type="submit">Add</button></form>
    <div className="record-list">{onboarding.map((o) => <div className="record-row" key={o.id}><span className={`task-status ${o.status === "done" ? "done" : ""}`} /><span className="record-copy"><strong>{o.title}</strong><small>{o.user_name}</small></span><span className="record-actions"><button className="text-button muted" type="button" onClick={() => void post("/hr/onboarding", { id: o.id, done: o.status !== "done" }, "Updated")}>{o.status === "done" ? "Reopen" : "Mark done"}</button></span></div>)}</div>

    <div className="section-heading task-heading"><div><p className="eyebrow">Documents</p><h2>{documents.length}</h2></div></div>
    <form className="task-composer workflow-composer" onSubmit={async (e) => { e.preventDefault(); if (await post("/hr/documents", docForm, "Document registered")) setDocForm({ user_id: "", title: "", doc_type: "contract" }); }}><Dropdown value={docForm.user_id} options={[{ value: "", label: "Employee" }, ...profiles.map((p) => ({ value: p.user_id, label: p.display_name }))]} onChange={(value) => setDocForm({ ...docForm, user_id: value })} ariaLabel="Employee" placeholder="Employee" /><input placeholder="Document title" value={docForm.title} onChange={(e) => setDocForm({ ...docForm, title: e.target.value })} required /><Dropdown value={docForm.doc_type} options={DOC_TYPE_OPTIONS} onChange={(value) => setDocForm({ ...docForm, doc_type: value })} ariaLabel="Document type" /><button className="primary-button" type="submit">Register</button></form>
    <div className="record-list">{documents.map((d) => <div className="record-row" key={d.id}><span className="record-avatar department-avatar">D</span><span className="record-copy"><strong>{d.title}</strong><small>{d.user_name} · {d.doc_type}</small></span></div>)}</div>
  </section>;
}

function SalesView() {
  const [leads, setLeads] = useState<Array<{ id: string; name: string; contact_email: string; source: string; status: string; owner_name: string }>>([]);
  const [opps, setOpps] = useState<Array<{ id: string; name: string; stage: string; amount: number; currency: string; company_name: string; owner_name: string }>>([]);
  const [companies, setCompanies] = useState<Array<{ id: string; name: string; industry: string; website: string }>>([]);
  const [leadForm, setLeadForm] = useState({ name: "", contact_email: "", source: "website" });
  const [oppForm, setOppForm] = useState({ name: "", amount: "", company_id: "" });
  const [companyForm, setCompanyForm] = useState({ name: "", industry: "" });
  const [message, setMessage] = useState("");

  const load = async () => {
    const [l, o, c] = await Promise.all([
      fetch(`${apiBase}/crm/leads`, { credentials: "include" }),
      fetch(`${apiBase}/crm/opportunities`, { credentials: "include" }),
      fetch(`${apiBase}/crm/companies`, { credentials: "include" }),
    ]);
    if (l.ok) setLeads(await l.json());
    if (o.ok) setOpps(await o.json());
    if (c.ok) setCompanies(await c.json());
  };
  useEffect(() => { void load(); }, []);
  const post = async (url: string, body: unknown, ok: string) => { const r = await fetch(`${apiBase}${url}`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) }); setMessage(r.ok ? ok : ((await r.json()).error?.message ?? "Action failed")); await load(); return r.ok; };
  const pipelineTotal = opps.filter((o) => o.stage !== "lost").reduce((sum, o) => sum + o.amount, 0);

  return <section className="content-wrap">
    <div className="page-heading"><div><p className="eyebrow">Sales & CRM</p><h1>Pipeline & customers.</h1><p className="lede">Leads, opportunities, and the companies you sell to. Open pipeline: {pipelineTotal.toLocaleString()}.</p></div></div>
    {message && <p className="inline-message">{message}</p>}
    <div className="section-heading task-heading"><div><p className="eyebrow">Opportunities</p><h2>{opps.length}</h2></div></div>
    <form className="task-composer workflow-composer" onSubmit={async (e) => { e.preventDefault(); if (await post("/crm/opportunities", { name: oppForm.name, amount: Number(oppForm.amount || 0), company_id: oppForm.company_id }, "Opportunity created")) setOppForm({ name: "", amount: "", company_id: "" }); }}><input placeholder="Opportunity name" value={oppForm.name} onChange={(e) => setOppForm({ ...oppForm, name: e.target.value })} required /><input placeholder="Amount" inputMode="decimal" value={oppForm.amount} onChange={(e) => setOppForm({ ...oppForm, amount: e.target.value.replace(/[^0-9.]/g, "") })} /><Dropdown value={oppForm.company_id} options={[{ value: "", label: "No company" }, ...companies.map((c) => ({ value: c.id, label: c.name }))]} onChange={(value) => setOppForm({ ...oppForm, company_id: value })} ariaLabel="Company" placeholder="No company" /><button className="primary-button" type="submit">Add</button></form>
    <div className="record-list">{opps.map((o) => <div className="record-row" key={o.id}><span className="record-avatar"><Target size={15} /></span><span className="record-copy"><strong>{o.name} · {o.currency} {o.amount.toLocaleString()}</strong><small>{o.company_name || "no company"}{o.owner_name ? ` · ${o.owner_name}` : ""}</small></span><div className="select-row"><Dropdown value={o.stage} options={OPP_STAGE_OPTIONS} onChange={(value) => void post("/crm/opportunities", { id: o.id, stage: value }, "Stage updated")} ariaLabel="Stage" /></div></div>)}</div>

    <div className="section-heading task-heading"><div><p className="eyebrow">Leads</p><h2>{leads.length}</h2></div></div>
    <form className="task-composer workflow-composer" onSubmit={async (e) => { e.preventDefault(); if (await post("/crm/leads", leadForm, "Lead created")) setLeadForm({ name: "", contact_email: "", source: "website" }); }}><input placeholder="Lead name" value={leadForm.name} onChange={(e) => setLeadForm({ ...leadForm, name: e.target.value })} required /><input placeholder="Email" value={leadForm.contact_email} onChange={(e) => setLeadForm({ ...leadForm, contact_email: e.target.value })} /><Dropdown value={leadForm.source} options={LEAD_SOURCE_OPTIONS} onChange={(value) => setLeadForm({ ...leadForm, source: value })} ariaLabel="Lead source" /><button className="primary-button" type="submit">Add</button></form>
    <div className="record-list">{leads.map((l) => <div className="record-row" key={l.id}><span className="record-avatar department-avatar">{l.name.slice(0, 1).toUpperCase()}</span><span className="record-copy"><strong>{l.name}</strong><small>{l.contact_email || "no email"} · {l.source}</small></span><div className="select-row"><Dropdown value={l.status} options={LEAD_STATUS_OPTIONS} onChange={(value) => void post("/crm/leads", { id: l.id, status: value }, "Lead updated")} ariaLabel="Lead status" /></div></div>)}</div>

    <div className="section-heading task-heading"><div><p className="eyebrow">Companies</p><h2>{companies.length}</h2></div></div>
    <form className="task-composer workflow-composer" onSubmit={async (e) => { e.preventDefault(); if (await post("/crm/companies", companyForm, "Company added")) setCompanyForm({ name: "", industry: "" }); }}><input placeholder="Company name" value={companyForm.name} onChange={(e) => setCompanyForm({ ...companyForm, name: e.target.value })} required /><input placeholder="Industry" value={companyForm.industry} onChange={(e) => setCompanyForm({ ...companyForm, industry: e.target.value })} /><button className="primary-button" type="submit">Add</button></form>
    <div className="record-list">{companies.map((c) => <div className="record-row" key={c.id}><span className="record-avatar department-avatar">{c.name.slice(0, 2).toUpperCase()}</span><span className="record-copy"><strong>{c.name}</strong><small>{c.industry || "—"}{c.website ? ` · ${c.website}` : ""}</small></span></div>)}</div>
  </section>;
}

function ITView() {
  const [tickets, setTickets] = useState<Array<{ id: string; type: string; title: string; priority: string; status: string; requester_name: string; assignee_name: string }>>([]);
  const [assets, setAssets] = useState<Array<{ id: string; name: string; category: string; serial_number: string; status: string; assignee_name: string }>>([]);
  const [articles, setArticles] = useState<Array<{ id: string; title: string; category: string; author_name: string }>>([]);
  const [ticketForm, setTicketForm] = useState({ type: "ticket", title: "", priority: "normal" });
  const [assetForm, setAssetForm] = useState({ name: "", category: "hardware", serial_number: "" });
  const [kbForm, setKbForm] = useState({ title: "", body: "", category: "general" });
  const [message, setMessage] = useState("");

  const load = async () => {
    const [t, a, k] = await Promise.all([
      fetch(`${apiBase}/itops/tickets`, { credentials: "include" }),
      fetch(`${apiBase}/itops/assets`, { credentials: "include" }),
      fetch(`${apiBase}/itops/kb`, { credentials: "include" }),
    ]);
    if (t.ok) setTickets(await t.json());
    if (a.ok) setAssets(await a.json());
    if (k.ok) setArticles(await k.json());
  };
  useEffect(() => { void load(); }, []);
  const post = async (url: string, body: unknown, ok: string) => { const r = await fetch(`${apiBase}${url}`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) }); setMessage(r.ok ? ok : ((await r.json()).error?.message ?? "Action failed")); await load(); return r.ok; };

  return <section className="content-wrap">
    <div className="page-heading"><div><p className="eyebrow">IT & Operations</p><h1>Service desk.</h1><p className="lede">Tickets, service and access requests, the asset registry, and the knowledge base.</p></div></div>
    {message && <p className="inline-message">{message}</p>}
    <div className="section-heading task-heading"><div><p className="eyebrow">Tickets</p><h2>{tickets.filter((t) => t.status !== "closed" && t.status !== "resolved").length} open</h2></div></div>
    <form className="task-composer workflow-composer" onSubmit={async (e) => { e.preventDefault(); if (await post("/itops/tickets", ticketForm, "Ticket created")) setTicketForm({ type: "ticket", title: "", priority: "normal" }); }}><input placeholder="What do you need?" value={ticketForm.title} onChange={(e) => setTicketForm({ ...ticketForm, title: e.target.value })} required /><Dropdown value={ticketForm.type} options={TICKET_TYPE_OPTIONS} onChange={(value) => setTicketForm({ ...ticketForm, type: value })} ariaLabel="Ticket type" /><Dropdown value={ticketForm.priority} options={TICKET_PRIORITY_OPTIONS} onChange={(value) => setTicketForm({ ...ticketForm, priority: value })} ariaLabel="Priority" /><button className="primary-button" type="submit">Raise</button></form>
    <div className="record-list">{tickets.map((t) => <div className="record-row" key={t.id}><span className="record-avatar"><LifeBuoy size={15} /></span><span className="record-copy"><strong>{t.title}</strong><small>{t.type.replace("_", " ")} · {t.priority} · by {t.requester_name}{t.assignee_name ? ` · ${t.assignee_name}` : ""}</small></span><div className="select-row"><Dropdown value={t.status} options={TICKET_STATUS_OPTIONS} onChange={(value) => void post("/itops/tickets", { id: t.id, status: value }, "Ticket updated")} ariaLabel="Ticket status" /></div></div>)}</div>

    <div className="section-heading task-heading"><div><p className="eyebrow">Assets</p><h2>{assets.length}</h2></div></div>
    <form className="task-composer workflow-composer" onSubmit={async (e) => { e.preventDefault(); if (await post("/itops/assets", assetForm, "Asset added")) setAssetForm({ name: "", category: "hardware", serial_number: "" }); }}><input placeholder="Asset name" value={assetForm.name} onChange={(e) => setAssetForm({ ...assetForm, name: e.target.value })} required /><Dropdown value={assetForm.category} options={ASSET_CATEGORY_OPTIONS} onChange={(value) => setAssetForm({ ...assetForm, category: value })} ariaLabel="Asset category" /><input placeholder="Serial #" value={assetForm.serial_number} onChange={(e) => setAssetForm({ ...assetForm, serial_number: e.target.value })} /><button className="primary-button" type="submit">Add</button></form>
    <div className="record-list">{assets.map((a) => <div className="record-row" key={a.id}><span className="record-avatar department-avatar">{a.name.slice(0, 2).toUpperCase()}</span><span className="record-copy"><strong>{a.name}</strong><small>{a.category}{a.serial_number ? ` · ${a.serial_number}` : ""}{a.assignee_name ? ` · ${a.assignee_name}` : ""}</small></span><span className="status-pill">{a.status.replace("_", " ")}</span></div>)}</div>

    <div className="section-heading task-heading"><div><p className="eyebrow">Knowledge base</p><h2>{articles.length}</h2></div></div>
    <form className="task-composer workflow-composer" onSubmit={async (e) => { e.preventDefault(); if (await post("/itops/kb", kbForm, "Article published")) setKbForm({ title: "", body: "", category: "general" }); }}><input placeholder="Article title" value={kbForm.title} onChange={(e) => setKbForm({ ...kbForm, title: e.target.value })} required /><input placeholder="Body" value={kbForm.body} onChange={(e) => setKbForm({ ...kbForm, body: e.target.value })} /><button className="primary-button" type="submit">Publish</button></form>
    <div className="record-list">{articles.map((a) => <div className="record-row" key={a.id}><span className="record-avatar department-avatar">KB</span><span className="record-copy"><strong>{a.title}</strong><small>{a.category}{a.author_name ? ` · ${a.author_name}` : ""}</small></span></div>)}</div>
  </section>;
}

function FinanceExtras() {
  const [invoices, setInvoices] = useState<Array<{ id: string; number: string; customer_name: string; amount: number; currency: string; status: string }>>([]);
  const [budgets, setBudgets] = useState<Array<{ id: string; name: string; amount: number; currency: string; period_start: string; period_end: string; spent: number }>>([]);
  const [purchases, setPurchases] = useState<Array<{ id: string; item: string; amount: number; currency: string; status: string; requester_name: string; approval_status: string }>>([]);
  const [invoiceForm, setInvoiceForm] = useState({ number: "", customer_name: "", amount: "" });
  const [budgetForm, setBudgetForm] = useState({ name: "", amount: "", period_start: "", period_end: "" });
  const [prForm, setPrForm] = useState({ item: "", amount: "", justification: "" });
  const [message, setMessage] = useState("");

  const load = async () => {
    const [inv, bud, pur] = await Promise.all([
      fetch(`${apiBase}/invoices`, { credentials: "include" }),
      fetch(`${apiBase}/budgets`, { credentials: "include" }),
      fetch(`${apiBase}/purchase-requests`, { credentials: "include" }),
    ]);
    if (inv.ok) setInvoices(await inv.json());
    if (bud.ok) setBudgets(await bud.json());
    if (pur.ok) setPurchases(await pur.json());
  };
  useEffect(() => { void load(); }, []);

  const post = async (url: string, body: unknown, ok: string) => {
    const response = await fetch(`${apiBase}${url}`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
    setMessage(response.ok ? ok : ((await response.json()).error?.message ?? "Action failed"));
    await load();
    return response.ok;
  };
  const money = (amount: number, currency: string) => `${currency} ${amount.toLocaleString(undefined, { minimumFractionDigits: 2 })}`;

  return <>
    {message && <p className="inline-message">{message}</p>}
    <div className="section-heading task-heading"><div><p className="eyebrow">Invoices</p><h2>{invoices.length}</h2></div></div>
    <form className="task-composer workflow-composer" onSubmit={async (event) => { event.preventDefault(); if (await post("/invoices", { number: invoiceForm.number, customer_name: invoiceForm.customer_name, amount: Number(invoiceForm.amount) }, "Invoice created")) setInvoiceForm({ number: "", customer_name: "", amount: "" }); }}><input placeholder="Invoice #" value={invoiceForm.number} onChange={(e) => setInvoiceForm({ ...invoiceForm, number: e.target.value })} required /><input placeholder="Customer" value={invoiceForm.customer_name} onChange={(e) => setInvoiceForm({ ...invoiceForm, customer_name: e.target.value })} required /><input placeholder="Amount" inputMode="decimal" value={invoiceForm.amount} onChange={(e) => setInvoiceForm({ ...invoiceForm, amount: e.target.value.replace(/[^0-9.]/g, "") })} required /><button className="primary-button" type="submit">Add</button></form>
    <div className="record-list">{invoices.map((inv) => <div className="record-row" key={inv.id}><span className="record-avatar department-avatar">#</span><span className="record-copy"><strong>{inv.number} · {money(inv.amount, inv.currency)}</strong><small>{inv.customer_name}</small></span><span className="status-pill">{inv.status}</span>{inv.status !== "paid" && <span className="record-actions"><button className="text-button" type="button" onClick={() => void post("/invoices/status", { id: inv.id, status: "paid" }, "Marked paid")}>Mark paid</button></span>}</div>)}</div>

    <div className="section-heading task-heading"><div><p className="eyebrow">Purchase requests</p><h2>{purchases.length}</h2></div></div>
    <form className="task-composer workflow-composer" onSubmit={async (event) => { event.preventDefault(); if (await post("/purchase-requests", { item: prForm.item, amount: Number(prForm.amount), justification: prForm.justification }, "Purchase request drafted")) setPrForm({ item: "", amount: "", justification: "" }); }}><input placeholder="Item" value={prForm.item} onChange={(e) => setPrForm({ ...prForm, item: e.target.value })} required /><input placeholder="Amount" inputMode="decimal" value={prForm.amount} onChange={(e) => setPrForm({ ...prForm, amount: e.target.value.replace(/[^0-9.]/g, "") })} required /><input placeholder="Justification" value={prForm.justification} onChange={(e) => setPrForm({ ...prForm, justification: e.target.value })} /><button className="primary-button" type="submit">Draft</button></form>
    <div className="record-list">{purchases.map((pr) => <div className="record-row" key={pr.id}><span className="record-avatar"><Wallet size={15} /></span><span className="record-copy"><strong>{pr.item} · {money(pr.amount, pr.currency)}</strong><small>by {pr.requester_name}{pr.approval_status ? ` · approval: ${pr.approval_status.replace("_", " ")}` : ""}</small></span><span className="status-pill">{pr.status}</span>{pr.status === "draft" && <span className="record-actions"><button className="text-button" type="button" onClick={() => void post(`/purchase-requests/${pr.id}/submit`, {}, "Submitted for approval")}>Submit</button></span>}</div>)}</div>

    <div className="section-heading task-heading"><div><p className="eyebrow">Budgets</p><h2>{budgets.length}</h2></div></div>
    <form className="task-composer" onSubmit={async (event) => { event.preventDefault(); if (await post("/budgets", { name: budgetForm.name, amount: Number(budgetForm.amount), period_start: budgetForm.period_start, period_end: budgetForm.period_end }, "Budget created")) setBudgetForm({ name: "", amount: "", period_start: "", period_end: "" }); }}><input placeholder="Budget name" value={budgetForm.name} onChange={(e) => setBudgetForm({ ...budgetForm, name: e.target.value })} required /><input placeholder="Amount" inputMode="decimal" value={budgetForm.amount} onChange={(e) => setBudgetForm({ ...budgetForm, amount: e.target.value.replace(/[^0-9.]/g, "") })} required /><DatePicker value={budgetForm.period_start} onChange={(value) => setBudgetForm({ ...budgetForm, period_start: value })} placeholder="Period start" ariaLabel="Period start" /><DatePicker value={budgetForm.period_end} onChange={(value) => setBudgetForm({ ...budgetForm, period_end: value })} placeholder="Period end" ariaLabel="Period end" /><button className="primary-button" type="submit">Add</button></form>
    <div className="record-list">{budgets.map((b) => <div className="record-row" key={b.id}><span className="record-avatar department-avatar">$</span><span className="record-copy"><strong>{b.name} · {money(b.amount, b.currency)}</strong><small>{b.period_start} → {b.period_end} · spent {money(b.spent, b.currency)}</small></span><span className={`status-pill ${b.spent > b.amount ? "leave-rejected" : ""}`}>{Math.round((b.spent / (b.amount || 1)) * 100)}%</span></div>)}</div>
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
    <form className="user-form" onSubmit={create}><p className="eyebrow">New project</p><div className="form-grid"><input placeholder="Key (e.g. MKT)" value={form.key} onChange={(event) => setForm({ ...form, key: event.target.value.toUpperCase().replace(/[^A-Z0-9-]/g, "") })} required /><input placeholder="Project name" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} required /><Dropdown value={form.lead_id} options={[{ value: "", label: "Project lead (optional)" }, ...users.map((user) => ({ value: user.id, label: user.display_name }))]} onChange={(value) => setForm({ ...form, lead_id: value })} ariaLabel="Project lead" placeholder="Project lead (optional)" /></div><input placeholder="Description (optional)" value={form.description} onChange={(event) => setForm({ ...form, description: event.target.value })} /><button className="primary-button" type="submit">Create project</button></form>
  </section>;
}

type CalView = "month" | "week" | "day" | "agenda";
const CAL_VIEWS: { key: CalView; label: string }[] = [
  { key: "month", label: "Month" }, { key: "week", label: "Week" }, { key: "day", label: "Day" }, { key: "agenda", label: "Agenda" },
];
const WEEKDAYS = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];
const MONTHS = ["January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"];
const startOfDay = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate());
const addDays = (d: Date, n: number) => new Date(d.getFullYear(), d.getMonth(), d.getDate() + n);
const sameDay = (a: Date, b: Date) => a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
const startOfWeek = (d: Date) => addDays(startOfDay(d), -((d.getDay() + 6) % 7)); // Monday-first
const pad2 = (n: number) => String(n).padStart(2, "0");
const dateKey = (d: Date) => `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())}`;
const eventOverlapsDay = (event: CalendarEvent, day: Date) => {
  const dayStart = startOfDay(day).getTime();
  const dayEnd = dayStart + 86400000;
  const start = new Date(event.starts_at).getTime();
  const end = new Date(event.ends_at).getTime();
  return start < dayEnd && end > dayStart;
};
const eventClass = (event: CalendarEvent) => (event.visibility === "private" ? "event-private" : "event-organization");
const hhmm = (iso: string) => { const d = new Date(iso); return `${pad2(d.getHours())}:${pad2(d.getMinutes())}`; };

function CalendarView() {
  const [events, setEvents] = useState<CalendarEvent[]>([]);
  const [view, setView] = useState<CalView>("month");
  const [cursor, setCursor] = useState<Date>(() => startOfDay(new Date()));
  const [message, setMessage] = useState("");
  const [showForm, setShowForm] = useState(false);
  const [form, setForm] = useState({ title: "", date: dateKey(new Date()), start: "09:00", end: "10:00", all_day: false, visibility: "organization" });
  const today = startOfDay(new Date());

  const load = async () => {
    const response = await fetch(`${apiBase}/calendar/events`, { credentials: "include" });
    if (response.ok) setEvents(await response.json());
  };
  useEffect(() => { void load(); }, []);

  const openComposer = (date: Date, hour?: number) => {
    setForm((current) => ({ ...current, date: dateKey(date), start: hour != null ? `${pad2(hour)}:00` : current.start, end: hour != null ? `${pad2(Math.min(hour + 1, 23))}:00` : current.end }));
    setShowForm(true);
  };

  const create = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const starts = form.all_day ? new Date(`${form.date}T00:00`) : new Date(`${form.date}T${form.start}`);
    const ends = form.all_day ? new Date(`${form.date}T23:59`) : new Date(`${form.date}T${form.end}`);
    const body = { title: form.title, all_day: form.all_day, visibility: form.visibility, starts_at: starts.toISOString(), ends_at: ends.toISOString() };
    const response = await fetch(`${apiBase}/calendar/events`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
    if (!response.ok) { setMessage((await response.json()).error?.message ?? "Could not create event"); return; }
    const data = await response.json() as { conflict: boolean };
    setMessage(data.conflict ? "Event created · note: it overlaps another of your events" : "Event created");
    setForm((current) => ({ ...current, title: "" }));
    setShowForm(false);
    await load();
  };

  const shift = (direction: number) => {
    if (view === "month") setCursor((d) => new Date(d.getFullYear(), d.getMonth() + direction, 1));
    else if (view === "week") setCursor((d) => addDays(d, 7 * direction));
    else setCursor((d) => addDays(d, direction)); // day + agenda step by day
  };

  const title = view === "month" ? `${MONTHS[cursor.getMonth()]} ${cursor.getFullYear()}`
    : view === "week" ? (() => { const start = startOfWeek(cursor); const end = addDays(start, 6); return `${MONTHS[start.getMonth()].slice(0, 3)} ${start.getDate()} – ${MONTHS[end.getMonth()].slice(0, 3)} ${end.getDate()}, ${end.getFullYear()}`; })()
    : view === "day" ? `${WEEKDAYS[(cursor.getDay() + 6) % 7]}, ${MONTHS[cursor.getMonth()]} ${cursor.getDate()}, ${cursor.getFullYear()}`
    : "Agenda";

  return <section className="content-wrap calendar-wrap">
    <div className="page-heading"><div><p className="eyebrow">Schedule</p><h1>Calendar.</h1><p className="lede">Shared events across the organization, with conflict detection on your own schedule.</p></div></div>
    {message && <p className="inline-message">{message}</p>}
    <div className="calendar-toolbar">
      <div className="calendar-nav">
        <button type="button" className="cal-today" onClick={() => setCursor(startOfDay(new Date()))}>Today</button>
        <button type="button" className="cal-arrow" aria-label="Previous" onClick={() => shift(-1)}><ChevronLeft size={17} /></button>
        <button type="button" className="cal-arrow" aria-label="Next" onClick={() => shift(1)}><ChevronRight size={17} /></button>
        <h2 className="calendar-title">{title}</h2>
      </div>
      <div className="calendar-toolbar-right">
        <div className="segmented" role="tablist" aria-label="Calendar view">
          {CAL_VIEWS.map((option) => (
            <button key={option.key} type="button" role="tab" aria-selected={view === option.key} className={`segmented-item ${view === option.key ? "active" : ""}`} onClick={() => setView(option.key)}>{option.label}</button>
          ))}
        </div>
        <button type="button" className="primary-button" onClick={() => openComposer(view === "month" ? today : cursor)}><Plus size={15} /> New event</button>
      </div>
    </div>

    {view === "month" && <CalendarMonth cursor={cursor} today={today} events={events} onPickDay={(day) => openComposer(day)} />}
    {(view === "week" || view === "day") && <CalendarTimeGrid days={view === "day" ? [cursor] : Array.from({ length: 7 }, (_, i) => addDays(startOfWeek(cursor), i))} today={today} events={events} onPickSlot={openComposer} />}
    {view === "agenda" && <CalendarAgenda events={events} today={today} />}

    {showForm && (
      <div className="cal-composer-backdrop" onMouseDown={() => setShowForm(false)}>
        <form className="cal-composer" onSubmit={create} onMouseDown={(event) => event.stopPropagation()}>
          <div className="cal-composer-head"><p className="eyebrow">New event</p><button type="button" className="icon-button" aria-label="Close" onClick={() => setShowForm(false)}>✕</button></div>
          <input className="cal-input" placeholder="Event title" value={form.title} onChange={(event) => setForm({ ...form, title: event.target.value })} required autoFocus />
          <label className="cal-field"><span>Date</span><DatePicker value={form.date} onChange={(value) => setForm({ ...form, date: value })} ariaLabel="Date" /></label>
          {!form.all_day && <div className="cal-field-row">
            <label className="cal-field"><span>Start</span><TimePicker value={form.start} onChange={(value) => setForm({ ...form, start: value })} ariaLabel="Start time" /></label>
            <label className="cal-field"><span>End</span><TimePicker value={form.end} onChange={(value) => setForm({ ...form, end: value })} ariaLabel="End time" /></label>
          </div>}
          <label className="cal-field"><span>Visibility</span><Dropdown value={form.visibility} onChange={(value) => setForm({ ...form, visibility: value })} ariaLabel="Visibility" options={[{ value: "organization", label: "Organization", color: "#2d6a58" }, { value: "private", label: "Private", color: "#6b4fa5" }]} /></label>
          <label className="cal-allday"><input type="checkbox" checked={form.all_day} onChange={(event) => setForm({ ...form, all_day: event.target.checked })} /> All day</label>
          <button className="primary-button cal-submit" type="submit">Add event</button>
        </form>
      </div>
    )}
  </section>;
}

function CalendarMonth({ cursor, today, events, onPickDay }: { cursor: Date; today: Date; events: CalendarEvent[]; onPickDay: (day: Date) => void }) {
  const days = useMemo(() => {
    const first = new Date(cursor.getFullYear(), cursor.getMonth(), 1);
    const gridStart = startOfWeek(first);
    return Array.from({ length: 42 }, (_, i) => addDays(gridStart, i));
  }, [cursor]);
  return <div className="cal-month">
    <div className="cal-month-header">{WEEKDAYS.map((weekday) => <div key={weekday} className="cal-weekday">{weekday}</div>)}</div>
    <div className="cal-month-grid">
      {days.map((day) => {
        const dayEvents = events.filter((event) => eventOverlapsDay(event, day)).sort((a, b) => a.starts_at.localeCompare(b.starts_at));
        const outside = day.getMonth() !== cursor.getMonth();
        return <div key={dateKey(day)} className={`cal-cell ${outside ? "outside" : ""} ${sameDay(day, today) ? "today" : ""}`} onClick={() => onPickDay(day)}>
          <span className="cal-daynum">{day.getDate()}</span>
          <div className="cal-cell-events">
            {dayEvents.slice(0, 3).map((event) => <button key={event.id} type="button" className={`cal-chip ${eventClass(event)}`} onClick={(clickEvent) => clickEvent.stopPropagation()} title={`${event.title} · ${hhmm(event.starts_at)}`}>{!event.all_day && <span className="cal-chip-time">{hhmm(event.starts_at)}</span>}<span className="cal-chip-title">{event.title}</span></button>)}
            {dayEvents.length > 3 && <span className="cal-more">+{dayEvents.length - 3} more</span>}
          </div>
        </div>;
      })}
    </div>
  </div>;
}

function CalendarTimeGrid({ days, today, events, onPickSlot }: { days: Date[]; today: Date; events: CalendarEvent[]; onPickSlot: (day: Date, hour: number) => void }) {
  const hours = Array.from({ length: 24 }, (_, i) => i);
  const scrollRef = useRef<HTMLDivElement>(null);
  useEffect(() => { if (scrollRef.current) scrollRef.current.scrollTop = 7 * 48; }, [days.length]);
  return <div className="cal-timegrid">
    <div className="cal-timegrid-head">
      <div className="cal-gutter-corner" />
      {days.map((day) => <div key={dateKey(day)} className={`cal-daycol-head ${sameDay(day, today) ? "today" : ""}`}><span className="cal-dayname">{WEEKDAYS[(day.getDay() + 6) % 7]}</span><span className="cal-daydate">{day.getDate()}</span></div>)}
    </div>
    <div className="cal-timegrid-body" ref={scrollRef}>
      <div className="cal-gutter">{hours.map((hour) => <div key={hour} className="cal-hour-label">{hour === 0 ? "" : `${pad2(hour)}:00`}</div>)}</div>
      {days.map((day) => {
        const timed = events.filter((event) => !event.all_day && eventOverlapsDay(event, day));
        return <div key={dateKey(day)} className="cal-daycol">
          {hours.map((hour) => <div key={hour} className="cal-slot" onClick={() => onPickSlot(day, hour)} />)}
          {timed.map((event) => {
            const start = new Date(event.starts_at); const end = new Date(event.ends_at);
            const startHours = sameDay(start, day) ? start.getHours() + start.getMinutes() / 60 : 0;
            const endHours = sameDay(end, day) ? end.getHours() + end.getMinutes() / 60 : 24;
            const top = startHours * 48; const height = Math.max(22, (endHours - startHours) * 48);
            return <div key={event.id} className={`cal-event ${eventClass(event)}`} style={{ top: `${top}px`, height: `${height}px` }} title={event.title}><strong>{event.title}</strong><span>{hhmm(event.starts_at)}–{hhmm(event.ends_at)}</span></div>;
          })}
        </div>;
      })}
    </div>
  </div>;
}

function CalendarAgenda({ events, today }: { events: CalendarEvent[]; today: Date }) {
  const upcoming = [...events].filter((event) => new Date(event.ends_at) >= today).sort((a, b) => a.starts_at.localeCompare(b.starts_at));
  const groups = new Map<string, CalendarEvent[]>();
  for (const event of upcoming) {
    const key = dateKey(new Date(event.starts_at));
    if (!groups.has(key)) groups.set(key, []);
    groups.get(key)!.push(event);
  }
  if (upcoming.length === 0) return <div className="cal-agenda-empty">No upcoming events.</div>;
  return <div className="cal-agenda">
    {[...groups.entries()].map(([key, dayEvents]) => {
      const date = new Date(`${key}T00:00`);
      return <div key={key} className="cal-agenda-day">
        <div className="cal-agenda-date"><strong>{date.getDate()}</strong><span>{WEEKDAYS[(date.getDay() + 6) % 7]}<small>{MONTHS[date.getMonth()].slice(0, 3)}</small></span></div>
        <div className="cal-agenda-events">{dayEvents.map((event) => <div key={event.id} className={`cal-agenda-row ${eventClass(event)}`}><span className="cal-agenda-dot" /><span className="cal-agenda-copy"><strong>{event.title}</strong><small>{event.all_day ? "All day" : `${hhmm(event.starts_at)} – ${hhmm(event.ends_at)}`}{event.creator_name ? ` · ${event.creator_name}` : ""}</small></span><span className="status-pill">{event.visibility}</span></div>)}</div>
      </div>;
    })}
  </div>;
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
      <Dropdown value={request.definition_id} options={[{ value: "", label: "Select workflow…" }, ...definitions.map((definition) => ({ value: definition.id, label: definition.name }))]} onChange={(value) => setRequest({ ...request, definition_id: value })} ariaLabel="Workflow" placeholder="Select workflow…" />
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
    {roles.length > 0 && <div className="rbac-panel"><div className="section-heading"><div><p className="eyebrow">Access control</p><h2>{roles.length} roles</h2></div></div><div className="record-list">{roles.map((role) => <div className="record-row" key={role.id}><span className="record-avatar">R</span><span className="record-copy"><strong>{role.name}</strong><small>{role.code} · {role.permissions.length} permissions</small></span></div>)}</div><form className="compact-form role-form" onSubmit={createRole}><p className="eyebrow">Create role</p><div className="form-grid"><input placeholder="Role code" value={roleForm.code} onChange={(event) => setRoleForm({ ...roleForm, code: event.target.value.toLowerCase().replace(/[^a-z0-9._-]/g, "-") })} required /><input placeholder="Role name" value={roleForm.name} onChange={(event) => setRoleForm({ ...roleForm, name: event.target.value })} required /></div><div className="permission-grid">{permissions.map((permission) => <label key={permission.code}><input type="checkbox" checked={roleForm.permissions.includes(permission.code)} onChange={(event) => setRoleForm({ ...roleForm, permissions: event.target.checked ? [...roleForm.permissions, permission.code] : roleForm.permissions.filter((code) => code !== permission.code) })} />{permission.code}</label>)}</div><button className="primary-button" type="submit">Create role</button></form><form className="compact-form role-form" onSubmit={assignRole}><p className="eyebrow">Assign role</p><div className="form-grid"><Dropdown value={assignment.user_id} options={[{ value: "", label: "Select person" }, ...users.map((user) => ({ value: user.id, label: user.display_name }))]} onChange={(value) => setAssignment({ ...assignment, user_id: value })} ariaLabel="Person" placeholder="Select person" /><Dropdown value={assignment.role_id} options={[{ value: "", label: "Select role" }, ...roles.map((role) => ({ value: role.id, label: role.name }))]} onChange={(value) => setAssignment({ ...assignment, role_id: value })} ariaLabel="Role" placeholder="Select role" /></div><button className="text-button" type="submit">Assign role <ArrowUpRight size={15} /></button></form></div>}
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

function SettingsView() {
  const [catalog, setCatalog] = useState<LookupCatalogItem[]>([]);
  useEffect(() => {
    fetch(`${apiBase}/lookups`, { credentials: "include" }).then((response) => (response.ok ? response.json() : [])).then(setCatalog).catch(() => { /* offline */ });
  }, []);
  const editable = [...catalog].filter((item) => item.editable).sort((a, b) => a.title.localeCompare(b.title));
  const locked = catalog.filter((item) => !item.editable);
  return <section className="content-wrap">
    <div className="page-heading"><div><p className="eyebrow">Configuration</p><h1>Settings.</h1><p className="lede">Manage the dropdown lists used across the workspace. Changes apply to everyone in your organization.</p></div></div>
    {catalog.length > 0 && editable.length === 0 && <p className="lede" style={{ marginTop: 40 }}>You don't have permission to edit any lists yet. Ask an administrator for access.</p>}
    <div className="settings-grid">
      {editable.map((item) => <LookupEditor key={item.category} item={item} />)}
    </div>
    {locked.length > 0 && <div className="settings-locked"><p className="eyebrow">Managed by other roles</p><div className="settings-locked-list">{locked.map((item) => <span key={item.category} className="settings-locked-item">{item.title}<small>{item.permission}</small></span>)}</div></div>}
  </section>;
}

function LookupEditor({ item }: { item: LookupCatalogItem }) {
  const [options, setOptions] = useState<LookupOption[]>([]);
  const [label, setLabel] = useState("");
  const [color, setColor] = useState("#2d6a58");
  const [message, setMessage] = useState("");
  const load = async () => { const response = await fetch(`${apiBase}/lookups/${item.category}`, { credentials: "include" }); if (response.ok) setOptions((await response.json()).options); };
  useEffect(() => { void load(); }, []);
  const add = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!label.trim()) return;
    const response = await fetch(`${apiBase}/lookups/${item.category}`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ label, color }) });
    if (response.ok) { setLabel(""); setMessage(""); await load(); } else setMessage((await response.json()).error ?? "Could not add option");
  };
  const remove = async (id: string) => { const response = await fetch(`${apiBase}/lookup-options/${id}`, { method: "DELETE", credentials: "include" }); if (response.ok) await load(); };
  return <div className="settings-card">
    <div className="settings-card-head"><h3>{item.title}</h3><span className="settings-count">{options.length}</span></div>
    <ul className="settings-options">
      {options.map((option) => <li key={option.id} className="settings-option">
        <span className="dropdown-swatch" style={{ background: option.color || "var(--faint)" }} />
        <span className="settings-option-label">{option.label}</span>
        <code>{option.value}</code>
        <button type="button" className="icon-button danger" aria-label={`Remove ${option.label}`} onClick={() => void remove(option.id)}><Trash2 size={14} /></button>
      </li>)}
    </ul>
    {message && <p className="form-error">{message}</p>}
    <form className="settings-add" onSubmit={add}>
      <input className="cal-input" placeholder="Add option…" value={label} onChange={(event) => setLabel(event.target.value)} />
      <input className="settings-color" type="color" value={color} onChange={(event) => setColor(event.target.value)} aria-label="Color" />
      <button type="submit" className="icon-button add" aria-label="Add option"><Plus size={16} /></button>
    </form>
  </div>;
}

function Metric({ label, value, change, detail, warning = false }: { label: string; value: string; change: string; detail: string; warning?: boolean }) {
  return <div className="metric-row">
    <div><p className="metric-label">{label}</p><strong className="metric-value">{value}</strong></div>
    <div className={`metric-change ${warning ? "warning" : ""}`}>{change}<small>{detail}</small></div>
  </div>;
}

export default App;
