# Design System: Corporate Event Ticketing

**Project ID:** local-phase1-react-spa

## 1. Visual Theme & Atmosphere

The product is a precise internal operations console for corporate event ticketing. The physical scene is an activity admin or HR user working under normal office lighting on a laptop, while check-in staff use the same product on a tablet at an event entrance with people waiting. This forces a light, high-clarity theme with strong hierarchy, compact controls, and readable status signals.

The desired lane is a mature Linear + Stripe inspired product surface: calm structure, crisp borders, restrained color, dense forms and tables, and confidence through behavior. It must not feel like a pale grey card pile. Use visual weight, section topology, sticky navigation, row density, and state vocabulary to separate primary work from supporting verification.

## 2. Color Palette & Roles

The palette stays restrained and cool, but avoids near-black blocks and near-white glare. Chroma steps are intentionally lower than the previous pass so navigation, status, and login surfaces read as one product system instead of separate visual temperatures.

- **Operations Canvas** (`oklch(0.958 0.007 238)`): Cool tinted app background that separates the workspace from panels without glare.
- **Command Surface** (`oklch(0.985 0.004 238)`): Main content surfaces, form bodies, tables, and ticket detail regions.
- **Inset Surface** (`oklch(0.938 0.008 238)`): Sidebars, segmented controls, empty states, skeletons, and secondary tool strips.
- **Raised Surface** (`oklch(0.972 0.005 238)`): Interactive rows, tickets, API entries, and result blocks.
- **Deep Ink** (`oklch(0.30 0.022 245)`): Primary text, table body, button labels, QR blocks, and high-priority metadata.
- **Quiet Slate** (`oklch(0.50 0.025 245)`): Helper copy, descriptions, secondary labels, table metadata, and inactive navigation.
- **Rule Line** (`oklch(0.84 0.014 238)`): Borders, dividers, table row rules, and QR frame edges.
- **Operations Blue** (`oklch(0.44 0.078 250)`): Primary actions, active navigation, focus rings, and selected rows.
- **Blue Wash** (`oklch(0.912 0.019 250)`): Selected backgrounds and informational tints.
- **Verified Green** (`oklch(0.39 0.078 153)`): Confirmed eligibility, active tickets, successful check-in, and passing readiness.
- **Green Wash** (`oklch(0.925 0.023 153)`): Successful status backgrounds.
- **Queue Amber** (`oklch(0.48 0.086 78)`): Waitlists, duplicate scans, warnings, and zero-match previews.
- **Amber Wash** (`oklch(0.936 0.032 82)`): Warning backgrounds.
- **Conflict Red** (`oklch(0.46 0.096 28)`): Ineligible booking, failed request, invalid token, destructive or failed states.
- **Red Wash** (`oklch(0.935 0.028 28)`): Error backgrounds.

## 3. Typography Rules

Use `Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif`. Typography is product-grade, not editorial: compact headings, strong label weight, and predictable type sizes. Body copy stays under 75ch where it is explanatory; tables and dense metadata can be wider. Letter spacing is always `0`.

## 4. Component Stylings

- **Shell:** Desktop uses a persistent left navigation, a bounded central workspace, and a quiet API activity rail. Under 1240px the API rail moves below content. Under 900px the shell becomes a stacked product workspace with horizontal-safe navigation.
- **Navigation:** Active route uses a filled wash, a clear icon container, and enough contrast to survive scanning. User and Admin areas stay visually distinct through labels and context blocks, not separate apps.
- **Buttons:** 3-4px radius, stable height, Lucide icons, visible focus rings, and consistent disabled treatment. Primary actions use Operations Blue. Secondary actions use neutral surfaces. Icon-only buttons require an accessible label on the button.
- **Panels and Rows:** Avoid same-weight card piles and pill-heavy framing. Use crisp 3-4px corners for bounded tools, rows for repeated event and ticket items, and full-width context bands for workspace state. Do not nest decorative cards.
- **Forms:** Group fields by intent, keep labels visible, mark required fields, and place readiness or helper strips near the action they affect. Date, number, select, and textarea controls retain native behavior.
- **Tables:** Dense rows, sticky-feeling header hierarchy, clear hover state, selected row state, and horizontal overflow inside `.table-scroll` only.
- **Status Vocabulary:** Badges pair text with semantic tints for OK, warning, fail, info, and neutral. Alerts use the same tone system and live regions.
- **Ticket QR:** QR is a stable square with a stronger frame and adjacent operational metadata. Raw signed tokens never appear in ordinary UI or API activity.
- **Check-in Result:** Success and duplicate states must be visually distinct at a glance, with the duplicate path showing first redemption metadata.

### Canonical Controls & Surfaces

`apps/web` uses installed shadcn primitives as the base vocabulary. Feature code should use app shared composites instead of raw component classes: `Field`, `TextareaField`, `SelectField`, `Alert`, `StatusBadge`, `EmptyState`, `Kpi`, `ResponsiveTable`, and `DangerZonePanel`. These wrappers preserve product-specific names while delegating control semantics, keyboard focus, labels, and ARIA behavior to shadcn primitives.

Use `Button` for all ordinary actions and choose variants by intent: default for primary workflow advancement, outline for secondary utility, ghost for quiet navigation, and destructive for high-risk actions. Use `Card` or a shared surface wrapper for bounded panels and context bands. Direct `.button`, `.field`, `.alert`, and feature-local table primitives are not part of the design system. Tables should enter through `ResponsiveTable` so horizontal overflow remains confined to the table region.

The semantic tokens in `src/styles.css` map shadcn variables to the operations palette: primary is Operations Blue, muted and secondary are calm utility surfaces, destructive is Conflict Red, card is Command Surface, and ring is the shared focus color. New pages should extend this token vocabulary, not introduce parallel neutral, black, or decorative palettes.

## 5. Layout Principles

The app should scan from role context to current task to operational proof. Page headers expose route purpose and control signals. Context bands summarize identity, capacity, attendance, or eligibility before the user reaches forms and tables. The main content grid uses 12 columns on desktop, collapses cleanly at tablet size, and keeps fixed-format elements such as QR codes, KPIs, icon buttons, and step numbers stable.

## 6. Production Page Standards

- **Local SSO:** Login is a real access surface, not a marketing hero. It shows app readiness, role groups, and the cookie security note with restrained hierarchy.
- **Employee Events:** The employee home is a calendar-first discovery surface with day, week, and month views, defaulting to week. Event cards emphasize poster, time, location, registration deadline, booking state, and one clear action while hiding internal IDs and technical eligibility detail. Confirmed registrations expose a quiet icon-only calendar export action that downloads a standard `.ics` file without raw internal IDs, ticket IDs, employee IDs, or signed tokens. The events page does not render ticket QR codes; it keeps exploration and scheduling separate from entry.
- **Employee Tickets:** Ticket selection and QR display must preserve token redaction while making the check-in handoff obvious. Current active tickets appear first as a focused ticket pass with poster, time, location, QR, and a concise entry instruction. Ticket timelines group registrations by event date and hide ticket IDs, employee IDs, zero companion counts, and internal metadata from ordinary employee views. Unavailable, not-open, expired, or revoked tickets use a single clear readiness state instead of showing unusable QR.
- **Admin Events:** Publish readiness, zero-match eligibility, capacity, and audit consequence must be visible before submit. Current or actionable activities are selected and listed before passive archived work.
- **Check-in:** Online scanner flow is scan-first: the current event is auto-selected, the QR camera has an alignment frame, scanned tokens submit immediately with debounce, and manual token entry remains as fallback. PWA/offline scanner redesign is deferred; the existing offline route remains separate.
- **Registration Governance:** Waitlist, active-ticket governance, cancellation, and revocation rows are ordered by needs-attention before passive records.
- **Reports and Audit:** Tables prioritize scanability, filtering, aggregation, and metadata detail without exposing unnecessary personal data. Waitlist, low-attendance, stale, conflict, failed, or retryable rows should surface before passive rows and include direct workspace links where safe.
- **Demo Control Panel:** The AC-9 flow is preserved as a practical verification tool with current-first ticket QR handoff, scan-first check-in wording, visible API activity, real data artifacts, and optional debug-clock time travel.
- **Responsive and Accessibility:** Verify 375px, 768px, 1024px, and 1440px. Maintain WCAG AA contrast, visible keyboard focus, reduced motion support, and no incoherent text overlap.
