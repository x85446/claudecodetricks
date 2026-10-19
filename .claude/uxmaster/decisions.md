# UX decisions

- 2026-10-16: Conductor Settings retains its header utility and left navigation. Editors use bounded widths; collection Add actions sit beside headings, item Edit actions sit beside names, and overdrive metadata and editor actions stay grouped with their window. This keeps short content and its controls together on wide displays.

- 2026-10-16: Configuration collections use draft-only Add/Edit dialogs with Save-to-commit-and-close, Cancel/Close/Escape-to-discard, keyboard focus containment and return focus. Breadcrumbs retain parent identity; browser and application Back restore prior locations; Settings returns to its entry page with automation drafts intact. Shared definitions expose usage and reject invalid deletion atomically.

- 2026-10-16: Subscription panels share one 800px boundary. Named-definition rows group Duplicate, Delete, Edit beside the name and Add beside the section heading. Overdrive rows show the name and used target percentage on one line; dialogs contain schedule details and preserve underlying page scroll on opening and closing.

- 2026-10-16: Duplicate and Delete use text-link styling on semantic buttons across collections, selectors and dialogs. Edit and Add use outlined buttons. Mouse hover adds an underline to links and accent feedback to buttons without changing their geometry; keyboard focus remains visible.

- 2026-10-16: Named Power Bands form a shared definition collection. Each subscription selects exactly one, defaulting to editable Universal power band. The editor exposes assigned subscriptions and an unsaved-draft simulator; referenced deletion requires reassignment.

- 2026-10-16: Projects anchor the portal. All projects opens a project workspace with Plans, Automations and Activity. Queued, active and archived plans share the Plans tab; queue counts deep-link into the selected project. Settings remains global and returns to the originating project tab.

- 2026-10-16: Preview 24 uses compact rows across plans, automations, activity and settings collections; project tabs share a 1000px width, settings retain 800px panels, and editors and simulators retain comfortable spacing.

- 2026-10-16: All projects uses a six-column sortable semantic table. Tags live in revisioned project configuration; multiple filters match any tag and persist alongside sorting in the URL. Enrolled identifies configuration ownership independently of enablement. Planned excludes queued/running plans. Activity links to the recorded plan milestone or assignment activity. Native Chrome verification covers sort toggles, tag save/reload/edit, OR/empty filters, plan drill-down, browser Back and a 320px viewport with table-contained scrolling.

- 2026-10-16: Plan views use All, Draft, Queued, Running, Blocked and Archived with one exclusive state per plan. All shows blocked work first; the project overview counts Blocked separately from Draft and Queued. Short badges, wrapped expandable reasons and a compact grid keep long operator messages inside the panel. Native Chrome verifies Filemaster counts 41/0/23/0/2/16, keyboard expansion and a 320px viewport without page overflow. The scheduler permits other eligible queued work while a plan is blocked; cross-plan prose dependencies are not enforced.
