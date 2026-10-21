# Design guidelines

These rules govern the Conductor web interface and its interactive previews. Use them when designing, implementing and reviewing screens. A preview demonstrates behavior; it does not establish an exception to these rules.

## Consistency across surfaces

Give the same action the same label, appearance, order and behavior everywhere. Apply this across collection pages, selectors, dialogs and nested editors. Reuse shared controls and layout patterns instead of styling each screen independently.

Consistency includes visual treatment. A semantic button styled as a text link still looks different from an outlined button.

**Violation example:** Duplicate and Delete appear as plain colored text beside a launch sequence, but as outlined buttons beneath the permission-preset selector. Both locations manage named definitions; Duplicate and Delete require the same text-link appearance in both.

## Actions and links

| Purpose | Treatment | Placement and behavior |
| --- | --- | --- |
| Duplicate | Text-link action | First in the item action group; opens an editable copy without saving it |
| Delete | Text-link action | Second; opens a confirmation that identifies the item and its references |
| Edit | Neutral outlined button | Third; opens an isolated draft |
| Add | Neutral outlined button labeled `+ Add <item>` | Beside the collection heading; opens an empty draft |
| Save | Filled primary button | Dialog footer, after Cancel; validates, saves and closes |
| Cancel / Close | Neutral outlined button | Discards the current draft and returns to its parent |
| Confirm deletion | Destructive text-link action labeled Delete | Confirmation footer; rejects invalid deletion without partial changes |
| Navigate | Text link | Moves to a destination, such as queued plans or account settings |

Use **Duplicate → Delete → Edit** in every item action group. Keep Add separate at the collection heading. When managing a selected definition beside a selector, keep the same three actions together and place `+ Add <item>` after them.

Use actual buttons for actions and links for destinations. Style Duplicate and Delete as text links everywhere, including dialogs and deletion confirmation. Keep them semantic buttons because they perform actions. Keep Edit and Add outlined. Keep secondary buttons consistent in height, padding, border, typography and focus treatment. Reserve filled emphasis for Save and other primary actions.

On mouse hover, outlined buttons use a subtle accent background and accent border; primary buttons change shade while retaining readable text. Duplicate, Delete and navigation links underline on hover. Hover does not move or resize controls. Preserve visible keyboard focus, and give disabled controls no active hover treatment. Keep actions usable without hover on touch devices.

Keep actions beside the content they affect. Do not push an Edit or Add button to the far edge of a wide page. Give repeated buttons accessible names that identify their item while retaining compact visible labels.

## Layout and collections

- Align subscription summaries, Named Launch Sequences and Overdrive windows to the same outer edges and 800px maximum content width. Let panels shrink to fit their container.
- Keep fields and action groups compact within those panels; matching panel widths do not require full-width controls.
- Use one heading for each section. Do not place an Overdrive heading immediately above another heading named Overdrive windows.
- Show each overdrive window as a single row: enable switch, name with target percentage, then Duplicate, Delete, Edit. Example: **Before Sunday reset (100%)**. Put recurrence, timezone and ending conditions in its editor.
- Keep collection summaries short. Put supplementary explanations behind a labeled disclosure; keep essential consequences and validation errors visible.
- Reflow controls at narrow widths, including 320px. Wrap groups without clipping labels, actions or fields. Single-line rows may wrap when space requires it.

Use compact bordered rows for plans, automations, activity and named configuration collections. Align status, name, summary and actions; keep supporting text to one short line where space allows. Let rows wrap on narrow screens. Keep project tabs aligned to a 1000px content width, and preserve comfortable spacing in editors and simulators.

## Dialogs and drafts

Use dialogs for Add, Duplicate and Edit of subscriptions, launch sequences, permission presets, overdrive windows and named time windows. Keep the parent collection visible behind the dialog.

- Opening a dialog or typing into it does not save configuration.
- Save validates the draft, commits one complete change, closes the dialog and updates the parent collection. Validation errors keep the draft open and explain the correction beside the affected field.
- Cancel, Close and Escape discard the current unsaved draft. They do not leave a partially created item behind.
- Preserve the underlying page's scroll position on open, save and dismiss. Position the dialog within the visible viewport; do not scroll the background to reveal it.
- Place initial focus inside the dialog, keep Tab and Shift+Tab inside it, and return focus to the invoking control or saved row without scrolling the background.
- Keep the title and closing controls reachable. Allow long dialog content to scroll while keeping its actions accessible.
- Use one dialog surface for nested definition editing. Returning from a permission-preset editor preserves the surrounding launch-sequence draft. An explicitly saved shared preset remains saved if the outer sequence draft is cancelled.
- Identify shared-definition usage before changes or deletion. Require reassignment of referenced definitions before deleting them.

## Navigation and ownership

Use All projects as the global overview. Its compact table has sortable Name, Queued, Draft, Running, Blocked, Enrolled and Activity columns. Name sorts alphabetically; counts sort numerically; Activity sorts by the latest recorded plan milestone or assignment start, with unavailable values last. Sort and tag selections persist in the URL. Queued counts link to the queue, Draft to work awaiting staging, Running to currently owned work, and Blocked to work awaiting attention. Project settings store lowercase tags; selecting multiple tags matches any selected tag. Enrollment and the enabled switch remain distinct. Keep the table horizontally scrollable within its own labeled region on narrow screens. Selecting a project opens its workspace with Plans, Automations and Activity tabs. Plans provides All, Draft, Queued, Running, Blocked and Archived filters. Each plan has one state; All includes every state and shows blocked work first. Use short status badges and expandable blocker explanations that wrap within the panel; queue counts open the corresponding project’s queued view. Plan details retain the project header and tabs. Scope activity to the selected project. Place Settings as a global labeled utility in the application header. Within Settings, order navigation as **AI Subscriptions → Named Power Bands → Named Time Windows → Named Automations**.

Show breadcrumbs that identify All projects, the selected project and its current tab or plan; Settings breadcrumbs identify the configuration section and owning subscription. Browser Back/Forward and application Back restore the corresponding location and selection. Settings returns to the page from which it was opened, preserving any automation draft.

Each AI Subscription owns its Named Launch Sequences and overdrive windows. A sequence name combines a fixed subscription-name prefix with an editable suffix, for example **Codex · build**. Derive that full name consistently in lists, selectors and results.

Use the same nouns in headings, buttons and errors. Use **Add time window**, not Add schedule, for a named time-window definition. Keep queued plan counts navigable to the corresponding project's plans.

## Named Power Bands

Manage reusable definitions under Settings → Named Power Bands. Ship the current weekly curve and short-window thresholds as **Universal power band**. Universal is editable and remains the default definition for new subscriptions.

Each AI Subscription references exactly one named power band through a single-select field. Several subscriptions may share a definition. Editing a shared definition affects its assigned subscriptions; usage, reset clocks, hard quotas and overdrive windows remain subscription-specific. Show the assigned name in the subscription summary and the affected subscriptions in the power-band editor.

Use Add, Duplicate, Edit and Delete with the shared collection and dialog patterns. Preserve Universal as the default definition. Require reassignment before deleting another definition that subscriptions use. Keep the power-band simulator inside the editor and evaluate the unsaved draft without changing live assignments.

## Power bars and simulators

- All power bars and their controls represent **percentage used**, increasing from 0% to 100%. Label remaining allowance separately and show weekly and five-hour reset information independently.
- Use dark green for the normal green band, orange for the band labeled Yellow, and red for the red band. Pair colors with text.
- Show a visible black current-use marker on every sample power bar.
- During active overdrive, show bright green from 0% through its target-used percentage. Any normal green beyond that target remains dark green. At or above the applicable target, retain target-length bright green and show **✓ Overdrive achieved**. Eligibility still follows the actual evaluator.
- Animate only the unfilled words **OVERDRIVE active**, briefly and with reduced-motion support. Keep bars and backgrounds still. Inactive overdrive uses neutral grey treatment.
- Mark testers as samples that do not change live settings or launch work. Use the available configuration; do not invent enabled accounts or windows.
- Keep the standalone policy tester free of subscription, overdrive and admission controls. Give elapsed time and usage independent sliders and publish both values. Preserve exact zero with a green current-sample label and a visible black marker, even when the configured green range has zero width.
- Present admission results once in a table in subscription and automation testers. Highlight only the selected eligible row in light green. Put **Would launch:** and its reason in that row; do not repeat the winner in a separate banner. Keep waiting and secondary rows neutral, with clear reasons.
- Show only relevant sample controls: unrestricted Keep it working needs no day/hour inputs; scheduled automations use day/hour controls; overdrive uses an available-window selector and a percentage-used slider.

## Review checklist

Review the same action across at least one collection and one dialog, rather than judging either screen alone.

- Do Duplicate, Delete, Edit and Add use the prescribed appearance and order everywhere?
- Do related panels share aligned outer edges, with compact controls inside?
- Does opening and dismissing an editor leave the background at the same scroll position?
- Do Save, Cancel, Close and Escape obey the draft lifecycle, including nested editing?
- Do Back, Forward, breadcrumbs and Settings return preserve context?
- Can a keyboard user reach every control, see focus and leave each dialog?
- Do controls remain usable at 320px without clipping or relying on hover?
- Do samples use real available definitions, correct percentage units and one highlighted result?

Record observed violations in `.claude/uxmaster/findings.md`. Keep a finding open until its interaction and visual treatment have been verified.

## Activation and field help

Use accessible binary switches for activation and checkboxes for independent selections such as weekdays. Disabled content uses muted, readable styling and compact detail disclosures. Keep its name, edit action and re-enable switch visible; preserve child settings. The dashboard owns project activation. Subscription cards and editors expose the same saved switch state. Failed or conflicting saves show the authoritative state and retain recoverable drafts.

Place labeled ⓘ buttons beside technical fields and concepts. Help opens by pointer or keyboard, fits a 320px viewport, dismisses with Escape or outside click, and restores focus without moving the background. Essential consequences and validation errors stay visible. Mark subscription fields Required, Optional or Defaulted according to validation. Keep common choices prominent and group optional overrides under Advanced; opaque account IDs belong in read-only Diagnostics.

Named Automations hold shared rules. Project attachments own local activation and order. Show Used by and the scope of shared edits. Distinguish Remove from project from Delete; offer Duplicate for independent rules and block referenced deletion.
