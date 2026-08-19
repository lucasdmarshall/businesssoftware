# Language and Design System

## Product Naming

The software does not have a fixed product name yet.

For design and architecture documents, refer to the product simply as:

> **Name**

Because this is a software product that will be sold and installed for different companies, the final customer-facing name and branding will remain configurable or undecided until a later product-branding phase.

## Technology Language Direction

### Production Backend

- Language: Go
- Responsibility: business logic, APIs, authentication, RBAC enforcement, workflows, database transactions, audit logs, licensing, backups, and integrations

### Desktop Client

- Shell: Tauri
- Responsibility: cross-platform desktop packaging, secure native integrations, local application lifecycle, and distribution

### Main Frontend

- Framework: React
- Language: TypeScript
- Responsibility: production UI, forms, tables, dashboards, navigation, accessibility, and general business screens

### Database

- PostgreSQL
- Responsibility: company system of record, transactional data, permissions, workflow history, audit records, and reporting data

### LukeLang

LukeLang is an owner-maintained strategic technology for selected reactive UI and specialized modules. It will be compiled and bundled into the product so that customers do not need to install or maintain LukeLang themselves.

Potential use cases include:

- Reactive dashboards
- Live analytics
- Workflow state interfaces
- Realtime notifications
- Attendance monitoring
- High-frequency visual updates

LukeLang code must be compiler-verified and covered by examples and automated tests before inclusion in a production release.

## Typography

### Primary System Font

Use **Plus Jakarta Sans** as the primary system font across the product.

Use the font consistently for:

- Navigation
- Dashboard headings
- Body text
- Tables
- Forms
- Buttons
- Tabs
- Notifications
- Empty states
- Dialogs

Font weights should be used with restraint. Prefer clear hierarchy through size, weight, spacing, and color rather than excessive bold text.

## Theme Support

The application must support both light and dark themes from the beginning.

Theme selection should be persisted per user or device and should be ready to support system preference detection later.

### Dark Theme

Primary background:

```text
#222323
```

The dark theme should use carefully layered dark surfaces and readable muted text. Avoid excessive pure black areas and avoid harsh white text where a softer high-contrast color is more comfortable.

### Light Theme

The light theme should use a warm white background rather than an aggressive, stark pure white.

The exact warm-white token may be refined during visual implementation, but it should feel soft, calm, and slightly warm while maintaining strong readability.

Example direction:

```text
Warm background: #FAF9F6 or a closely related warm-white token
```

The final token should be validated with contrast checks and real screens.

## Theme Switcher

The theme switcher should be docked at the bottom-right corner of the screen.

Requirements:

- Clearly discoverable
- Compact and unobtrusive
- Available from the main application shell
- Works in both light and dark themes
- Does not cover important actions or content
- Supports keyboard and assistive technology access
- Displays the current theme state

The switcher may appear as a small floating control, but it should not visually become a large card or permanent dashboard element.

## Design Approach

The interface should follow a **Material Design approach** for interaction patterns and familiar controls.

Use Material-inspired patterns including:

- Pills
- Tabs
- Icons
- Menus
- Tooltips
- Drawers
- Dialogs where necessary
- Clear focus and pressed states
- Predictable form and table behavior

Material Design is an interaction and usability reference, not a requirement to copy default Material styling. The product should retain its own restrained visual identity.

## Visual Design Direction

The visual language is:

> **GitBook-inspired, Swiss-style, borderless, cardless, boxless, and clean.**

The interface should prioritize typography, alignment, spacing, hierarchy, and content over decorative containers.

### Default Rules

- Prefer whitespace and alignment over borders.
- Prefer typography and spacing over cards.
- Prefer flat content sections over nested boxes.
- Use clear visual hierarchy with restrained color.
- Use icons to support recognition, not to decorate every label.
- Keep navigation structured and calm.
- Make dense enterprise information easy to scan.
- Keep layouts consistent across departments and modules.

## Cards, Boxes, and Containers

Cards, boxes, and bordered containers should not be the default layout pattern.

Use them only where they provide a necessary function, such as:

- A clear grouping of unrelated content
- A form or workflow that needs strong boundaries
- A warning or critical status
- A permission-sensitive action
- A modal or temporary interaction
- A dashboard metric requiring visual separation
- A drag-and-drop region
- A selected or active state

Boxes and cards may appear on hover or focus when they help communicate interactivity. Hover states should be subtle and should not cause layout shift.

## Interaction and State Design

Every interactive control should have clear states:

- Default
- Hover
- Focus
- Pressed
- Selected
- Disabled
- Loading
- Success
- Warning
- Error

State changes should be communicated through a combination of color, iconography, text, and position. Color must not be the only indicator, especially for finance, HR, attendance, and approval workflows.

## Component Direction

The initial component system should include:

- App shell
- Sidebar and navigation groups
- Top bar
- Breadcrumbs
- Tabs
- Pills and status labels
- Buttons
- Icon buttons
- Inputs and selects
- Date and time controls
- Tables
- Filters
- Command/search interface
- Toasts and notifications
- Drawers
- Dialogs
- Empty states
- Loading states
- Error states
- Activity timeline
- Approval stepper

Components must support both themes, keyboard navigation, responsive layouts, and permission-aware visibility.

## Accessibility

Accessibility is part of the design system, not a later enhancement.

The interface should provide:

- Keyboard navigation
- Visible focus states
- Sufficient contrast
- Semantic headings
- Accessible names for icons
- Screen-reader-friendly controls
- Reduced-motion support
- Non-color status indicators
- Readable table and form layouts

## Design System Summary

```text
Name                  = Product name remains undecided
System Font           = Plus Jakarta Sans
Themes                = Light and Dark
Dark Background       = #222323
Light Background      = Warm white, not stark pure white
Theme Switcher        = Docked at bottom-right
Interaction Approach  = Material Design patterns
Visual Style          = GitBook + Swiss style
Containers            = Borderless, cardless, boxless by default
Cards / Boxes         = Only when necessary; may appear on hover
Primary UI            = Clean, structured, typography-led enterprise UI
```
