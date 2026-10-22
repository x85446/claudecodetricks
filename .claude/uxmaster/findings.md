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
