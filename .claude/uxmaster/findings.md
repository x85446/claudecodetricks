# UXMASTER findings — claudecodetricks


### F1 — tutorial runtime ignores FORCE_COLOR / CLICOLOR_FORCE
- surface: skills/tutorial/lib run.sh, tutorial.sh (colour decision)
- kind: platform-convention
- severity: low
- evidence: matrix row 6 — `printf 'q\n' | FORCE_COLOR=1 ./run.sh | cat | grep -c $'\033'` → 0 on a fixture project (gaur, 2026-10-05); run.sh has no --color flag, so the force variables are the only way to keep colour through a pipe
- fix: one ladder, `tut_color_on` in box.sh, used by both scripts: NO_COLOR → off, FORCE_COLOR / CLICOLOR_FORCE≠0 → on, TERM=dumb → off, non-TTY stdout → off
- rule: color.md §1 rung 4, §7 row 6
- status: fixed

### F2 — `↑` and `↯` have no ASCII fallback
- surface: tutorial.sh failure and Ctrl-C notices
- kind: platform-convention
- severity: low
- evidence: `LANG=C LC_ALL=C TUT_AUTO=1 ./run.sh 2` prints `↑ exited 1` as raw UTF-8 bytes while the box frame correctly drops to `+ - |`
- fix: `tut_utf8` in box.sh (UTF-8 locale and TERM not dumb) picks `↑ ↯` or `^ !`
- rule: color.md §5
- status: fixed

### F3 — menu numbers and keys in bold yellow are faint on light themes
- surface: run.sh menu, tutorial.sh `[N]` step headers
- kind: ui
- severity: low
- evidence: `python3 ptyrun.py 80 'q\n' ./run.sh` — numbers rendered `ESC[1;33m`; meaning is carried by the key column and `[N]` position, not by colour alone
- fix: none; the user chose bold yellow for numbers and keys when making the roles the standard (gaur step 5)
- rule: color.md §4 (yellow on white)
- status: deferred (user-chosen role; meaning survives with colour stripped)

### F4 — the command line uses the success hue (green) instead of the identifier role
- surface: tutorial.sh `$ cmd` line
- kind: ui
- severity: low
- evidence: `TUT_AUTO=1 ./run.sh 1` in a pty — `ESC[1;32m  $ echo hi`; the `$ ` prefix carries the meaning
- fix: none; bold green for the command is part of the role standard the user approved (gaur step 5); cyan is already the frame colour
- rule: color.md §3
- status: deferred (user-chosen role)

### F5 — failure notices go to stdout, not stderr
- surface: tutorial.sh `exited N`, `tut_done` failure row
- kind: platform-convention
- severity: low
- evidence: `TUT_AUTO=1 ./run.sh 2 2>/dev/null` still shows `^ exited 1`
- fix: none; the notice is narration interleaved with the step's own output, and splitting streams would reorder it under any pipe; the exit code (1) already reports the failure to scripts
- rule: color.md §3 (error role on stderr)
- status: deferred (tutorial narration must stay in order with command output)

### F6 — Settings interrupts the operational navigation
- surface: Conductor preview navigation
- kind: ux
- severity: medium
- evidence: Executed conductor-preview-v19.html in jsdom and clicked Settings, AI Subscriptions, Named Launch Sequence editing, and Projects. Main navigation reads Projects, Settings, Activity; configuration sits between the two operational destinations. The user reports difficulty with Settings placement. Browser tab binding times out at Emulation.setFocusEmulationEnabled, so this evidence covers DOM behavior and information architecture only. Verification: executed v20 in jsdom, navigated Projects and Activity into header Settings and back, and checked the ordered AI Subscriptions/Budgets/Named Time Windows rail and its active state. Scope: interactive preview DOM; responsive layout and browser accessibility remain unverified.
- fix: Place a text-labeled Settings utility action in the Conductor header, separate from Projects and Activity. Give Settings its own navigation ordered AI Subscriptions, Budgets, Named Time Windows; use a compact left rail on wide screens and stacked navigation on narrow screens.
- rule: —
- status: fixed

### F7 — Launch sequence names allow subscription identity to drift
- surface: Conductor Settings / AI Subscriptions / Named Launch Sequence editor
- kind: ux
- severity: medium
- evidence: Clicked the Codex sequence editor in the running jsdom preview; the editable name field contains the full string Codex · build. It allows the subscription-owned prefix to be edited independently of the owning subscription. Verification: exercised v20 suffix editing, whitespace trimming, empty-name focus, duplicate creation, subscription rename, stable automation references, subscription scoping, and persisted reload in jsdom. Scope: interactive preview naming behavior.
- fix: Derive the prefix from the subscription name and display it as fixed text beside the editable sequence suffix. Use the derived full name in every picker, simulator, and project summary; subscription rename updates the prefix while sequence IDs and references stay stable.
- rule: —
- status: fixed

### F8 — Named Time Windows actions use a different noun
- surface: Conductor Settings / Named Time Windows
- kind: ux
- severity: low
- evidence: Clicked Named Time Windows in the running jsdom preview; its actions read Add schedule, Delete schedule, and Save schedule alongside Reset-relative window. Verification: opened v20 Named Time Windows, added a window, and checked Add time window, Delete time window, Save time window, and Time window name labels in jsdom. Scope: interactive preview labels.
- fix: Use Add time window, Delete time window, Save time window, and Time window name throughout the window editor and its validation messages.
- rule: —
- status: fixed

### F9 — Settings actions lack proximity to the content they control
- surface: Conductor Settings / AI Subscriptions / Named Launch Sequences and overdrive windows
- kind: ux
- severity: medium
- evidence: The user reports excessive content width and distant Add/Edit actions in the rendered v20 preview. Executed v20 in jsdom, opened AI Subscriptions, edited the Codex launch sequence, returned, and expanded its first overdrive window. The sequence heading/Add pair and sequence name/Edit pair use space-between rows; overdrive separates the name/status, schedule/target, and editor actions into independent full-width groups. These interactions confirm the affected controls and grouping, but do not measure pixel distance. Browser binding times out at Emulation.setFocusEmulationEnabled.
- verification: Executed v21 in jsdom: header Add and per-item Edit remain local after rerenders; overdrive Edit opens its controls, target saves to 75% used, enable toggles persist, Add creates a disabled window, and sequence creation/return retain the compact grouping. Computed styles bound the editor to 960px and sequence/overdrive sections to 800px. Existing naming, navigation, persistence and powerbar marker checks pass. Browser pixel layout, narrow-screen geometry and WCAG remain unverified.
- fix: Preserve the Settings workspace and navigation. Bound the editor content width, place the sequence Add action beside its heading, and place Edit in a compact action column next to the sequence name. Group each overdrive window's name, enabled state, status, schedule, and target within a compact card; align its editing actions with its fields. Keep simulator bars within the same content boundary and stack controls on narrow screens.
- rule: —
- status: fixed

### F10 — Sequence creation persists before the user saves
- surface: Conductor Settings / AI Subscriptions / Named Launch Sequences
- kind: mistake
- severity: high
- evidence: Executed v21 in jsdom, clicked Add launch sequence, and observed the persisted profile count increase immediately. Clicked the sequence Back control without saving; the subscription list contains the new sequence. Editing and saving an existing sequence leaves the user on the full editor page.
- fix: Open Add and Edit in a modal dialog over the owning subscription list. Keep edits in an isolated draft; validate and commit only on Save, then close and update the originating list. Cancel and Escape discard the draft. Restore focus to the invoking action or the saved row. Apply the same lifecycle to overdrive and named time windows.
- rule: —
- status: fixed (interactive preview; production implementation remains planned)

- verification: Chrome v22 verifies sequence Save closes, Escape discards and returns focus, Tab/Shift+Tab wrap, and the dialog fits at 320px. DOM checks cover subscription, sequence, preset, overdrive and named-window drafts, validation, save/cancel and atomic deletion guards.

### F11 — Settings navigation loses the user's origin
- surface: Conductor operational navigation and Settings
- kind: ux
- severity: medium
- evidence: Executed v21 in jsdom: opened Settings from Activity, then clicked its Back control; the result is Projects. Navigating subscription and sequence views leaves window.history.length at 1 and the URL unchanged. The user reports getting lost and a Back control that does not perform the expected return. This evidence covers application DOM/history behavior, not native browser Back interaction.
- fix: Preserve the originating page and selected subscription/project, show a breadcrumb for the current location, and provide a Back control that restores the prior location. Keep create/edit dialogs inside their parent location. Integrate browser history when the preview host permits it; maintain equivalent in-preview Back navigation when it does not.
- rule: —
- status: fixed (interactive preview; production implementation remains planned)

- verification: Chrome v22 verifies Activity → Settings → browser Back returns to Activity. DOM checks cover browser Forward, queue/plan routes, contextual breadcrumbs, Settings entry return and preservation of an unsaved automation draft.

### F12 — Named-definition actions use inconsistent visual treatments
- surface: Conductor Settings / Named Launch Sequences and launch-sequence permission presets
- kind: ui
- severity: medium
- evidence: The user-supplied screenshot shows Duplicate and Delete as plain colored text in the background sequence row and outlined buttons inside the open sequence editor. The exercised preview exposes these same collection and nested preset actions. Matching action order does not resolve the visual mismatch.
- fix: Use neutral outlined Duplicate, Delete and Edit buttons in that order in every named-definition action group. Keep Add beside the collection heading, or after the action group for a selected preset. Reserve text links for navigation and destructive emphasis for the deletion confirmation.
- rule: docs/design-guidelines.md — Actions and links
- status: fixed (interactive preview)

- verification: Opened the served preview in Chrome and edited Codex · build. The permission-preset dialog, sequence collection and overdrive rows display neutral outlined Duplicate, Delete and Edit buttons using the shared cp-button style.


## Javelina verification — 2026-10-21

### F13 — Subscription setup exposes implementation details before useful choices
- surface: subscription summary and editor
- kind: ux; severity: high; status: fixed
- evidence: user screenshots and isolated Chrome fixture localhost:8421.
- fix: AI tool, host program detection, account/usage status and power band lead; literal arguments and technical controls live in Advanced; account IDs are read-only Diagnostics.
- verification: Codex detection resolves an absolute host path; Verify executes help only; missing custom path receives a remedy; tool switching preserves explicit overrides and clears readiness. Cancel discards drafts. Required/Optional/Defaulted labels, no JSON requirement, popover Escape/focus/background-scroll and 320px dialog geometry pass. Touch opens overdrive help; keyboard opens/dismisses subscription help.

### F14 — Activation controls are scattered and disabled content looks active
- surface: dashboard, subscription summary/editor, overdrive parent/window, automation attachment, named window
- kind: ux; severity: high; status: fixed
- fix: activation uses role=switch, dashboard owns project activation, disabled rows/panels are muted and compact with visible names/Edit/re-enable controls. Weekdays remain selection checkboxes.
- verification: pointer/Space/touch toggle fixture projects; Save/Edit preserves activation. Subscription card and editor share state. Disabling overdrive preserves its enabled child and collapses its tester; re-enabling restores it. Attachments remain independently enabled/disabled across two projects. Regression API checks reject conflicting writes; browser retains an automation draft on conflict. A stale polling response cannot replace a newer config revision.

### F15 — Power-band tester mixes unrelated account and scheduler decisions
- surface: Named Power Bands editor
- kind: ux; severity: high; status: fixed
- fix: four independent sample inputs, policy-only API, six five-hour threshold points and separate current-sample label.
- verification: Chrome shows exact day 0 / 0% as green with visible marker and unchanged zero thresholds; 2h 33m and 51% remain independent. No subscription, overdrive, warning or decision table. Executed UI regression rejects out-of-order/closed-dialog responses. Visual review of /tmp/javelina-final-powerband.png confirms readable controls and unclipped markers.

### F16 — Overdrive completion loses its visual outcome
- surface: subscription and automation testers
- kind: ui; severity: medium; status: fixed
- fix: applicable target completion uses a green target region and checkmarked achieved label; real admission keeps its hard gates.
- verification: targets 60 and 100 pass below/exact/above/rewind fixture cases. Chrome at 60 and 100 shows achieved and accurate marker. Disabled subscription remains denied. /tmp/javelina-overdrive-achieved.png passes independent visual review.

### F17 — Reusing automation rules requires copies with unclear ownership
- surface: global Named Automations and project Automations
- kind: ux; severity: high; status: fixed
- fix: shared named definitions, local attachments, Used by, shared-edit explanation, independent Duplicate and guarded Delete.
- verification: browser creates a definition, attaches it to Sample A and B disabled, enables A independently, duplicates and deletes the unused copy, prevents referenced deletion, detaches/re-attaches B disabled, and preserves a stale draft. Bounded worker tests prove distinct assignment/occurrence identities; checkpoint tests prove active snapshots survive shared edits.

Final repeat sweep: no additional findings across the covered dashboard, subscription, power-band, time-window, overdrive and shared-automation flows. Nested help, cancel/reopen, disabled-parent/enabled-child and 320px subscription layout are exercised. Global fixture admission stays disabled; no model work runs. Production localhost:8420 and user configuration remain unchanged.

### F18 — Plan and archive navigation waits for global history
- surface: `/plan` and `/archive`
- kind: ux; severity: high; status: fixed
- evidence: live Caiman and archived Javelina requests exceeded 20 seconds without response bytes while handlers decoded the 160 MB shared hook log.
- fix: filesystem plan details render immediately; tool history and token usage load separately with retry feedback, cancellation and one concurrent history reader. Live activity keeps its polling nodes and scroll position. Archive redirects preserve history requests.
- verification: installed localhost:8420 returns Caiman in 0.281 seconds and Javelina in 0.278 seconds. Chrome displays initial goals and requirements while history loads, then the completed history; requirement details remain interactive. Handler regression covers blocked history, missing history, unlabelled completion pairs, cancellation and archive redirects.

## Run-report comparison — 2026-10-22

### F19 — Missing usage hides the accounting section
- surface: archived Gemsbok report; Albatross and Waterbuck comparison
- kind: mistake
- severity: high
- evidence: Chrome reports history loaded on all three pages. Gemsbok has no Token spend section; Albatross has 236 requests with a missing coordinator transcript; Waterbuck has five requests. The token renderer returns without an empty-state explanation when Messages is zero.
- fix: Always show accounting availability, capture coverage and missing sources. Distinguish unavailable usage from zero usage, and preserve a durable plan-scoped record across providers and archives.
- rule: —
- status: open

### F20 — Completion cards count lanes without naming that unit
- surface: plan/archive completion cards
- kind: mistake
- severity: high
- evidence: Fully loaded Gemsbok shows 0 DONE and 1 RECORDED OPEN while all four clickable requirements show DONE and its archive records complete with merged PR28. Albatross shows one queued lane despite all 20 requirements showing DONE. Source counts row states rather than requirement completion.
- fix: Separate plan outcome, task completion, validation completion and lane state. Close recorded lanes from authoritative terminal events; label unknown termination explicitly.
- rule: —
- status: open

### F21 — Requirement detail omits recorded acceptance evidence
- surface: requirement detail and expanded coordinator
- kind: ux
- severity: high
- evidence: Clicking Albatross requirement 3 shows a result NOTE. Clicking Gemsbok requirement 1 and expanding its coordinator shows the contract and checkmark without the actual result; its saved log contains 24-PDF acceptance, artifact identity, 299-case validation and merge evidence. Waterbuck requirement 4 carries a note mixing steps 4, 6 and 7.
- fix: Store and display evidence per requirement and attempt: result, command, timestamp, exit status, artifact and source revision. Keep intended acceptance separate from observed proof.
- rule: —
- status: open

### F22 — Session cost and plan usage use different scopes
- surface: Waterbuck Token spend
- kind: ux
- severity: high
- evidence: Chrome shows 893k context, 725 generated tokens, five requests and $784.89 SESSION COST together. Supporting prose says the dollar amount includes unrelated work in the same sessions. The report provides no coverage measure reconciling these totals with its 22h7m59s execution span.
- fix: Group plan-attributed usage separately from whole-session cost. Show the attribution interval, coverage and estimated versus provider-reported amounts beside each total.
- rule: —
- status: open

### F23 — Time gaps have severity without a recorded cause
- surface: Waterbuck timeline
- kind: ux
- severity: medium
- evidence: After history loads, the report labels a 7h21m30s interval a severe gap under Where the time actually went. The report does not identify whether that interval represents an operator wait, a schedule, a running external process or missing capture.
- fix: Show elapsed, observed execution, blocked, queued, paused and unobserved time separately. Reserve failure severity for evidence of a failure; show unknown intervals as unknown.
- rule: —
- status: open

## Named Automations audit — 2026-10-22

### F24 — Named Automations link cannot show the list while an editor is open
- surface: Settings / Named Automations
- kind: mistake
- severity: high
- evidence: Isolated preview (fixture revision 1). Opened Edit on "Run yellow (Codex)", renamed it, then selected Named Automations in the settings rail. The URL changed but the page still rendered the editor and 0 list rows; only Cancel revealed the list. The unsaved draft persists in sessionStorage, so a reload reopens the editor instead of the list.
- fix: Drive the editor from the URL (`automation=<id>`). The rail link always renders the complete list; an unsaved draft appears there as a recoverable row with Resume and Discard. Save returns to the refreshed list; Cancel discards and returns. Back and forward restore the matching view.
- rule: —
- status: fixed
- verification: Headless Chrome on the isolated preview: with "Run yellow (Codex)" renamed in the editor, the rail link showed all automations, the row marked Unsaved changes with its draft name, and Resume restored the edit; Cancel kept the saved name; Back/Forward toggled list and editor; reload kept the draft.

### F25 — Save fails silently after an unrelated configuration change
- surface: automation editor Save
- kind: mistake
- severity: high
- evidence: With the editor open, a tag change on another project raised the revision from 1 to 2. Save then failed with "configuration revision conflict"; the message rendered at y=500 while the Save button sat at y=1296, so the user scrolled to Save sees nothing happen. The automation itself was unchanged by the other edit.
- fix: Rebase the draft onto the latest configuration when the saved automation and its time window are unchanged since the draft began; report a real conflict beside the Save button and move focus to it.
- rule: —
- status: fixed
- verification: After a project tag change raised the revision, Save of a band change on "Run yellow (Codex)" persisted red and returned to the list. A same-automation conflict reports beside Save and focuses the message.

### F26 — One project's tab shows a different automation's unsaved editor
- surface: project Automations tab
- kind: mistake
- severity: high
- evidence: A draft opened from Settings binds to the first configured project (Filemaster). Filemaster's Automations tab then rendered the "Run yellow (Codex)" editor in place of its attachment list, while Filemaster's only attachment is "yellow go"; Cancel restored the list showing "yellow go". The editor heading showed the unsaved rename with no draft marker.
- fix: Render an editor only for the automation named in the URL. Label every unsaved name as a draft beside its saved name, and show saved names in lists, pickers and attached rows.
- rule: —
- status: fixed
- verification: Unsaved rename of "Green and go" showed Unsaved changes in the editor, the Named list and Symmail's attached row, which kept "Green and go"; project tabs render an editor only for the automation in the URL.

### F27 — Duplicate automation names are indistinguishable in pickers
- surface: project attachment picker, Named Automations list
- kind: ux
- severity: medium
- evidence: Duplicated "yellow go", renamed the copy to "yellow go" and saved. Symmail's picker listed two options with the identical text "yellow go" (different IDs); the settings list showed two identical rows.
- fix: When names collide, append a short stable ID to every affected label; warn in the editor that the name is shared.
- rule: —
- status: fixed
- verification: Two "Night owl" automations render as "Night owl · #07a933" and "Night owl · #b9ec1e" in the list, izcrOS picker and association cards; attaching the second stored definition b9ec1e…, disabled.

### F28 — Automation tester reports live project and account state instead of the rules
- surface: Try this automation
- kind: ux
- severity: high
- evidence: For "yellow go" (Claude build through Green → Codex fast through Yellow) every sample band, Green, Yellow and Red, produced "Conductor disabled" on both rows; switching the sample project to disabled izcrOS produced the same. The tester asks for a sample project, account availability, Project already running and Queued work available, none of which are automation rules.
- fix: Evaluate the draft rows for a hypothetical eligible project: one sample band input (plus the sample time for schedules and the overdrive state when honored), one outcome naming the matched row or the reason no row permits the band, and a per-band summary. Production admission keeps every live gate.
- rule: —
- status: fixed
- verification: Run yellow (Codex) (Green + Yellow): Green and Yellow would launch row 1, Red waits; Green and go waits for Yellow; yellow go launches row 1 for Green and row 2 for Yellow. Outcomes were identical with stale/blocked usage, a parked plan and zero saved projects; config.json and runtime.json bytes unchanged. Go tests killdeer-preview-* and killdeer-api-automation-preview cover the contract.

### F29 — Rule summary ignores unsaved band changes
- surface: automation editor summary
- kind: ux
- severity: medium
- evidence: Changed row 1 of "yellow go" from Green only to Green + Yellow + Red; the summary still read "Claude Code · build through green → …" until Save. Band values also appear as raw ids ("through yellow").
- fix: Recompute the summary on every draft change and name bands the way the row selector does ("Green + Yellow").
- rule: —
- status: fixed
- verification: Changing Green and go row 1 to Green + Yellow updated the summary to "Codex · build through Green + Yellow" and the Yellow row to Would launch before Save; the saved band stayed green.

### F30 — Managing which projects use an automation requires visiting every project
- surface: Named Automations, project Automations tab
- kind: ux
- severity: medium
- evidence: The only attach control is a per-project picker. Attaching "Green and go" to three projects took three project visits; the Named Automations list shows Used by as text with no way to change it.
- fix: Add a Project Associations screen: drag a project onto an automation to attach (disabled by default), drag it back to detach, with equivalent Attach/Detach controls for keyboard and touch, live status feedback, and both automation and project orientations.
- rule: —
- status: fixed
- verification: Real pointer drags (CDP drag interception) attached Symmail and Filemaster to Run yellow (Codex) disabled, a duplicate drop changed nothing, a drag back detached; Enter/Space/Enter attached, enabled and detached by keyboard; both orientations, the Symmail tab and Used by agreed after reload; a 320px viewport has no horizontal scroll.

### F31 — Opening an automation creates no history entry
- surface: automation editor, browser Back
- kind: platform-convention
- severity: medium
- evidence: Headless Chrome on the preview: from /settings?section=automations, Edit "yellow go" kept the same URL and history length 3. Back then left Settings for All projects, with no editor and no list, while the unsaved draft stayed in sessionStorage. The editor cannot be deep-linked or restored by Forward.
- fix: Give the editor its own URL (`automation=<id>` or `automation=new`) pushed on open, so Back returns to the list and Forward reopens the same draft.
- rule: HTML Living Standard session history (pushState); WCAG 2.2 SC 3.2.3 Consistent Navigation
- status: fixed
- verification: Opening an automation pushes `automation=<id>`; Back returned to the list without an editor and Forward reopened the same editor.
