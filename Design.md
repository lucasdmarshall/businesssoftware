# Design Notes

Living UI notes for the product. Canonical branding, typography, and layout
rules remain in [`Language-and-Design-System.md`](./Language-and-Design-System.md).
This file records **implemented control conventions** that screens must follow.

## Custom form controls (required)

Native browser form widgets are **not** used for select lists or date/time entry
in the authenticated app. Every screen must use the shared custom components:

| Control | Replaces | Location |
|---|---|---|
| **Dropdown** | `<select>` | `frontend/src/App.tsx` + `components.css` |
| **DatePicker** | `<input type="date">` | same |
| **TimePicker** | `<input type="time">` | same |

### Why

- Matches the GitBook / Swiss visual language (no OS-native chrome).
- Supports color swatches (leave types, statuses).
- Keyboard navigation, focus states, and light/dark themes stay consistent.
- Lookup lists from Settings (`lookup_options`) plug into the same Dropdown API.

### Rules for new UI

1. Do **not** add native `<select>` or `<input type="date|time">` in module screens.
2. Prefer `useLookup(category)` for organization-editable option lists.
3. Static enums (priority, scope, cadence, CRM stages) use shared option constants
   passed into `Dropdown`.
4. Date/time values stay ISO strings (`YYYY-MM-DD`, `HH:MM`) for API payloads.
5. Popovers close on outside click and Escape; triggers expose accessible names.

### Coverage

These controls are already wired across Work, Attendance, Leave, Schedule,
Calendar composer, Reports, Finance, HR, Sales, IT, Projects, Approvals,
People/RBAC, and Settings.

When building the next production-depth module, reuse these controls first —
do not invent a parallel picker.

## Related

- Interaction patterns and container rules: `Language-and-Design-System.md`
- Offline behaviour: `docs/OFFLINE_RULES.md`
