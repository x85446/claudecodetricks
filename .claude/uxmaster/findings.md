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
- status: fixed
- verification: Preview on :8481 with history: Gemsbok shows Tracking coverage complete and Usage with 70 requests (its coordinator ran in Codex; the rollouts were never read before), Albatross shows coverage partial with five captured team lanes (236 requests) and the coordinator lane missing because the hook log attributes no coordinator session, and the session count reads "0 of unknown". A plan with no usage keeps all three sections with an unavailable badge and "unknown, not zero". Go cases kr-coverage-missing and kr-coverage-providers cover absent, known-zero, partial, Codex, Claude, flat and teamed paths.

### F20 — Completion cards count lanes without naming that unit
- surface: plan/archive completion cards
- kind: mistake
- severity: high
- evidence: Fully loaded Gemsbok shows 0 DONE and 1 RECORDED OPEN while all four clickable requirements show DONE and its archive records complete with merged PR28. Albatross shows one queued lane despite all 20 requirements showing DONE. Source counts row states rather than requirement completion.
- fix: Separate plan outcome, task completion, validation completion and lane state. Close recorded lanes from authoritative terminal events; label unknown termination explicitly.
- rule: —
- status: fixed
- verification: Verified progress section (report_progress.go), preview :8482 in headless Chrome against the real archives: Gemsbok reads "Completed and archived", merge PR 28 at 399c3bd, Complete 4 of 4 requirements, Lanes 1 lane completed (closed by the plan's completion event); Albatross 20 of 20 requirements and 6 lanes completed, merge PR 19 at ea3929c; Waterbuck Executing, 11 of 15, coordinator lane active. Every count names its unit; an ended plan's lane without a terminal observation reads "Unknown end". TestKrProgressComplete and TestKrProgressPartial pass. The superseded done/recorded open/queued cards in renderTimelinePage are removed by the layout step.

### F21 — Requirement detail omits recorded acceptance evidence
- surface: requirement detail and expanded coordinator
- kind: ux
- severity: high
- evidence: Clicking Albatross requirement 3 shows a result NOTE. Clicking Gemsbok requirement 1 and expanding its coordinator shows the contract and checkmark without the actual result; its saved log contains 24-PDF acceptance, artifact identity, 299-case validation and merge evidence. Waterbuck requirement 4 carries a note mixing steps 4, 6 and 7.
- fix: Store and display evidence per requirement and attempt: result, command, timestamp, exit status, artifact and source revision. Keep intended acceptance separate from observed proof.
- rule: —
- status: fixed
- verification: Validation evidence section (report_evidence.go) on preview :8482: Waterbuck requirement 4 lists only its own four step-4 passages, and steps 6 and 7 hold their own; Gemsbok requirement 1 shows its imported acceptance line and plan-level evidence exposes 259/299-case counts, the 24-PDF scanner proof, build hash 2bcb15dc… and merge PR 28 at 399c3bd; requirements 2–4 read "Checked, no proof recorded". Acceptance text sits in its own disclosure. Observed items show result, command, exit status, time, artifact link, revision and environment; imported prose reads "Imported · Result not observed". TestKrEvidenceBinding and TestKrEvidenceFreshness pass. The requirement-grid note that merged steps is replaced by the layout step.

### F22 — Session cost and plan usage use different scopes
- surface: Waterbuck Token spend
- kind: ux
- severity: high
- evidence: Chrome shows 893k context, 725 generated tokens, five requests and $784.89 SESSION COST together. Supporting prose says the dollar amount includes unrelated work in the same sessions. The report provides no coverage measure reconciling these totals with its 22h7m59s execution span.
- fix: Group plan-attributed usage separately from whole-session cost. Show the attribution interval, coverage and estimated versus provider-reported amounts beside each total.
- rule: —
- status: fixed
- verification: Preview on :8481, Waterbuck with history: Usage shows 430 plan-attributed requests (5 Claude Code, 425 Codex) in an exact block with the cost tile "unknown", the $784.89 figure under "Whole-session cost (not this plan's bill)" for one session, Codex allowance readings in their own block, the attribution window stated, and tool and wake-up breakdowns labelled estimated. Go cases kr-usage-scopes and kr-usage-dedup cover scope separation, price provenance and duplicate requests.

### F23 — Time gaps have severity without a recorded cause
- surface: Waterbuck timeline
- kind: ux
- severity: medium
- evidence: After history loads, the report labels a 7h21m30s interval a severe gap under Where the time actually went. The report does not identify whether that interval represents an operator wait, a schedule, a running external process or missing capture.
- fix: Show elapsed, observed execution, blocked, queued, paused and unobserved time separately. Reserve failure severity for evidence of a failure; show unknown intervals as unknown.
- rule: —
- status: fixed
- verification: The plan report's Time section (killdeer lifecycle) partitions Waterbuck's elapsed time on the :8483 preview into execution 1d 12h 51m (83%) and unobserved 7h 32m (17%), and lists the 7h 21m interval as Unobserved with "No record covers this interval; it is unknown, not a failure", the ownership state and the observed work on either side. TestKrTimeUnknown shows the same Waterbuck-shaped interval relabelled Blocked on operator only after a recorded blocker covers it. The old severe-gap stat and list remain on the page until the layout step removes them (F52).

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

## Accounting sections review — 2026-10-22

### F32 — Complete and captured badges fail text contrast in light mode
- surface: plan report Tracking coverage, Usage and Delivery state badges
- kind: a11y
- severity: medium
- evidence: Headless Chrome on :8481 (Albatross, Gemsbok, Waterbuck): badge text in --good (#059669) on the white section background measured 3.77:1 at 11.5px; in dark mode the partial badge on --warn-bg measured 4.48:1.
- fix: Badge text uses --text on the tinted state background (--good-bg, --warn-bg), with the state color as the border; the word carries the state.
- rule: WCAG 2.2 SC 1.4.3 Contrast (Minimum)
- status: fixed
- verification: Re-measured on :8481: the lowest text contrast in the three sections is 5.02:1 light and 5.04:1 dark; summaries are 24px tall.

### F33 — The plan page has no main landmark or skip link
- surface: /plan and /archive pages
- kind: a11y
- severity: medium
- evidence: Headless Chrome on :8481: document.querySelectorAll('main,[role=main]') is empty on Albatross, Gemsbok and Waterbuck, no in-page skip link exists, and the report sections are regions outside any main landmark.
- fix: Wrap the page content in <main> and add a skip link to it, during the report layout step.
- rule: WCAG 2.2 SC 1.3.1 Info and Relationships; SC 2.4.1 Bypass Blocks
- status: fixed
- verification: Layout step, preview :8484 (worktree build) in headless Chrome: Albatross, Gemsbok and Waterbuck each have one <main id="main" tabindex="-1">; the first Tab on Waterbuck lands on "Skip to the plan report" and Enter moves focus to main. TestKrReportNavigation asserts one main and the skip link as the first link on live, completed and archived pages; conductor.report-navigation checks it in Chrome.

### F34 — The superseded token panel repeats usage with a different scope
- surface: /plan and /archive, "Token spend" below the timeline
- kind: ux
- severity: medium
- evidence: Headless Chrome on :8481: after the Usage section, each page still renders the old Token spend panel (.tk-top) with its own totals and a "session cost" tile beside them, the presentation F22 describes.
- fix: Remove the writeTokenPanel call from the timeline renderer; the Usage section carries its lane, tool and wake-up tables.
- rule: —
- status: fixed
- verification: writeTokenPanel, tokenPanelCSS and their table helpers are removed; the three reference pages on :8484 carry no .tk-top, "Token spend" or table.tk, and Usage holds the lane, tool and wake-up tables. TestUsageSectionRendersLanesToolsAndWakeups and TestUsageIsUnknownWhenThereIsNothingMeasured assert the Usage equivalents; `iterate-run tokens` still prints its three tables from tokens.go.

### F35 — An unknown session denominator read as an unknown capture count
- surface: Tracking coverage counts, Albatross
- kind: ux
- severity: low
- evidence: Headless Chrome on :8481: the sessions tile read "unknown sessions captured" although the report knew no coordinator session was captured; only the total was unknown.
- fix: Show "0 of unknown" when the count is known and the denominator is not; keep "unknown" only before tool history loads.
- rule: —
- status: fixed
- verification: Albatross on :8481 reads "0 of unknown sessions captured · unavailable" with the reason beside it.

### F36 — Report sections keep their no-history state after history loads
- surface: /plan and /archive without history=1 (the default navigation)
- kind: mistake
- severity: high
- evidence: Headless Chrome on :8481, Gemsbok archive: after the status line changed to "Tool history and token usage loaded.", Usage still read "Plan usage is unavailable until tool history loads" and Tracking coverage still read "Usage is unavailable until tool history loads". The loader replaces only #plan-recorded-content, and the report sits before it.
- fix: When history loads, replace #plan-report with the response's #plan-report as well as #plan-recorded-content (conductor_web/detail.js), or render the report inside #plan-recorded-content.
- rule: —
- status: fixed
- verification: Headless Chrome on :8484, real Gemsbok archive without history=1, history request held by the DevTools Fetch domain: before release Usage and Tracking coverage read "until tool history loads"; after "Tool history and token usage loaded." both read complete, the opened "Requirement by requirement" and Time definitions stay open, and requirement 1 opened by Enter on its grid cell keeps its open evidence, focus and selected mark. detail.js matches disclosures by key rather than position and keeps the element at the viewport top in place. conductor.report-navigation repeats this on the fixture.

## Report integrity (killdeer integrity team) — 2026-10-22

### F40 — An earlier "partial" log line overrides a requirement's checked boxes
- surface: live Waterbuck requirement detail
- kind: mistake
- severity: high
- evidence: GET localhost:8420/plan for Waterbuck with history, `bdShow(4)`: the detail reads "REQUIREMENT 4 · COORDINATOR · ACTIVE" although the plan file has `- [x] 4.` in both Steps and Validation. The 07:00:30Z Status/Log line "step 4 partial: …" is the latest line the step-status pattern matches; the later "step 4 bake.cleanup.green rerun passed" does not match it, so the stale partial wins over the checkboxes.
- fix: Derive requirement state from the plan's checkboxes first and treat prose step reports as imported evidence for that requirement, not as state. A report only sets state on a plan that has no checkbox for it.
- rule: —
- status: fixed
- verification: Preview :8482 Waterbuck: Verified progress lists requirement 4 Complete ("validation box checked"); its "step 4 partial" passage appears under Validation evidence as an imported report ("Reports: partial"), followed by the later "rerun passed" passage. TestKrEvidenceBinding asserts the same on the fixture. The Requirements grid in timeline.go still applies the log override; the layout step points it at the progress model.

### F41 — Requirement cells open their detail by mouse only
- surface: plan/archive Requirements grid (`.bd-cell`)
- kind: a11y
- severity: high
- evidence: Headless Chrome on Waterbuck: each requirement cell is a `<div onclick="bdShow(N)">` with tabIndex -1 and no role, so Tab never reaches it and Enter/Space cannot open the detail that carries the step, its validation and its note.
- fix: Give every requirement a keyboard-operable disclosure (native `<details>`/`<summary>`) that holds its acceptance text and observed evidence; replace or convert the click-only cells.
- rule: WCAG 2.2 SC 2.1.1 Keyboard; SC 4.1.2 Name, Role, Value
- status: fixed
- verification: The requirement grid lives in Verified progress as <a class="bd-cell" href="#evidence-req-N" aria-label="Requirement N, <state>, lane <lane>">; state also shows as a shape (check, dot, cross, dashed border). Headless Chrome on :8484: Tab reaches all 15 Waterbuck cells with a 2px focus ring, Enter on Albatross cell 20 opens #evidence-req-20 and focuses its summary. Cells take their state from the progress model, so the stale "partial" override (F40) no longer reaches the grid; gantt steps link to their evidence instead of opening merged notes.

### F42 — Success green text in the report sections is below 4.5:1 in the light theme
- surface: Verified progress and Validation evidence (Complete badge, complete count, Available label, passed results)
- kind: a11y
- severity: medium
- evidence: Headless Chrome on preview :8482 (Waterbuck, light scheme), computed colours: `--good` #059669 text on the white surface measures 3.77:1 for the 11px "Complete" badge, the 18px "11 of 15" count and the "Available" label. The dark scheme passes (5.44:1 and up).
- fix: Scope a text green to both sections, #047857 in the light scheme and `--good` in the dark scheme, and use it for every green text rule; borders keep the page token.
- rule: WCAG 2.2 SC 1.4.3 Contrast (Minimum)
- status: fixed
- verification: Re-measured on the rebuilt preview: lowest light-scheme ratios 5.02:1 (badges, state label), 4.83:1 (counts), 5.85:1 (result labels); dark scheme 4.60:1 and up.

## Lifecycle report sections — 2026-10-22

### F48 — Section status lines repeat their own label
- surface: plan report Time and Attempts and rework sections (Waterbuck, Albatross, Gemsbok)
- kind: mistake
- severity: low
- evidence: Headless Chrome on the :8483 preview: the Time status read "Partial. Partial: this execution has no run record; …" on all three reference pages.
- fix: The availability label is the sentence's subject; the note follows as its own sentence.
- rule: —
- status: fixed
- verification: Waterbuck, Albatross and Gemsbok now read "Partial. This execution has no run record; …"; data-status carries the state for scripts.

### F49 — One section shows two clocks
- surface: Time intervals (Before/After) and Ownership handoffs (gates, release)
- kind: ux
- severity: medium
- evidence: Waterbuck on the preview: the interval heading read "2026-10-20 18:24 CDT to 2026-10-21 01:46 CDT" while its context lines read "coordinator ended 2026-10-20T23:24:48Z"; handoff gates and the release line used raw RFC 3339 UTC as well.
- fix: Carry instants as structured fields in the model and render every one as a local <time> element with the exact instant in datetime.
- rule: WCAG 2.2 SC 3.2.4 Consistent Identification
- status: fixed
- verification: The preview's Waterbuck Time section and the TestKrHandoffResume fixture page render every instant as "YYYY-MM-DD HH:MM <zone>" in a <time datetime> element; the JSON keeps RFC 3339.

### F50 — Unknown figures read as zero
- surface: Time partition and lane-time rows, Ownership summary
- kind: mistake
- severity: high
- evidence: Preview: Waterbuck's partition showed "Blocked on operator 0s 0%" for an execution whose legacy history carries no blocker capture; lane-time rows read "none observed" in the label and "not captured" in the value; Ownership showed "0 ownership holds, 0 blockers" and "Last heartbeat: no owner" where ownership was never recorded; intervals before the first recorded ownership event said "no owner held the plan".
- fix: Zero-time causes read "none recorded"; lane-time carries state measured, none or unknown; Ownership reads "not recorded" without ownership events; ownership before the first event is "not recorded by then".
- rule: —
- status: fixed
- verification: Preview Waterbuck (history loaded) shows "Blocked on operator none recorded —", "Waiting on dispatched subagents none observed", "Last heartbeat not recorded" and "Recorded no ownership or blocker events"; before history loads, tool lanes read "unknown — tool history is not loaded"; TestKrTimeUnknown asserts "ownership not recorded by then".

### F51 — Plain-language counts and kinds
- surface: Attempts and rework, Time lane-time
- kind: ui
- severity: low
- evidence: Fixture pages: "3 retrys", "1 lane; 1 span" for one Conductor attempt, the internal attempt kind "pair" beside every attempt, and "(partial)" on token totals whose only gap was cost.
- fix: Pluralize correctly, name each lane kind's unit (calls, units, attempts, intervals), hide the default kind, mark tokens partial only when input or output tokens are missing.
- rule: —
- status: fixed
- verification: TestKrReworkInterrupted's page reads "3 retries, 1 restart", "1,200 tokens", and "Conductor attempts 1 attempt".

### F52 — Severe-gap judgment still beside the Time section
- surface: plan/archive pages: "N severe gap(s)" stat, the "severe gap (5m+)" legend entry, red gantt gap markers, "Where the time actually went"
- kind: ux
- severity: medium
- evidence: Preview Waterbuck shows "1 SEVERE GAP" and the "Where the time actually went" list above the Time section that labels the same 7h 21m interval Unobserved, unknown and not a failure.
- fix: Remove the severe-gap stat, legend entry and list, and draw gantt gaps in the neutral unobserved pattern; the Time section's intervals replace them.
- rule: —
- status: fixed
- verification: The severe-gap stat, legend entries, "Where the time actually went" list and red .gapmark markers are removed from the page; Waterbuck on :8484 shows the 7h 21m interval only under Time as Unobserved. The timeline caption says a gap between bars is not a failure and links to Time. SevereGap remains for `iterate-run timeline`'s terminal summary and timeBridge. TestKrReportNavigation asserts no "severe gap", gapmark or gap list on any page.

### F53 — Two elapsed figures disagree on a live plan
- surface: plan page runbox ("Recorded execution span" / "Running for") and the Time section
- kind: mistake
- severity: medium
- evidence: Preview Waterbuck (live): the runbox reads 44h12m13s, ending at the latest activity (14:36:27), while Time reads 1d 20h 23m, ending at the report time (14:47). Archived Albatross and Gemsbok agree because both end at Finished:.
- fix: One clock: remove the runbox or render it from the Time section's window (start basis, end basis, open flag).
- rule: —
- status: fixed
- verification: The runbox is removed; #plan-elapsed renders the Time section's clock ("Elapsed 1d 20h 55m from 2026-10-20 18:24 CDT, still open at 2026-10-22 15:19 CDT" on Waterbuck, "1h 18m" Executing: to Finished: on Albatross, matching Time). The gantt caption names its own range as first to last recorded activity, not elapsed time. TestElapsedPrefersExecutingOverStarted, ...FallsBackToEarliestActivity..., ...HandlesZeroConfirmedActivity and ...PrefersFinishedOverActivityForArchivedPlans assert the four clock cases.

### F54 — First-pass success is unknowable for CLI-run plans
- surface: Attempts and rework
- kind: ux
- severity: medium
- evidence: Preview: Waterbuck, Albatross and Gemsbok all show "Unavailable. No attempts are recorded for this execution." Only Conductor assignments become attempts; /iterate records none, though Albatross's team logs carry 14 validation assessments.
- fix: Have /iterate append attempt.started and attempt.finished per requirement through iterate-run record append (or an equivalent hook observation), so CLI runs report retries, recoveries and first-pass success.
- rule: —
- status: open (needs the iterate executor to record attempts)


## Report layout (killdeer layout) — 2026-10-22

### F56 — The execution timeline sits below eight report sections with no way to jump there
- surface: /plan and /archive (Albatross, Gemsbok, Waterbuck)
- kind: ux
- severity: medium
- evidence: Headless Chrome at 1280px on the :8484 preview of 2d1fa616 with history loaded: the Coordinator gantt heading sits at y=8385 of a 9873px Albatross page, y=6724 of 7478 on Gemsbok and y=8244 of 9260 on Waterbuck, after Verified progress, Validation evidence, Tracking coverage, Usage, Time, Ownership, Attempts and Delivery. At 320px Albatross is 22754px tall. The page has no in-page navigation.
- fix: Order the page header, outcome and evidence summary, execution timeline, then the breakdowns (Time, Ownership, Rework directly under the gantt, then Usage, Coverage, Delivery), with Conductor activity last in a bounded region; add an "On this page" link list after the header.
- rule: WCAG 2.2 SC 2.4.5 Multiple Ways (advisory for in-page navigation); SC 1.3.2 Meaningful Sequence
- status: fixed
- verification: :8484 at 1280px: Execution timeline heading at y=3043 of 9023 (Albatross), 2061 of 7072 (Gemsbok), 2594 of 7782 (Waterbuck); order header, On this page, Verified progress, Validation evidence, Execution timeline, Time, Ownership, Attempts, Usage, Coverage, Delivery, Conductor activity. TestKrReportNavigation and conductor.report-navigation assert the order.

### F57 — The gantt track collapses to zero width at 320px
- surface: Coordinator and Activity by team gantts
- kind: a11y
- severity: high
- evidence: Headless Chrome at 320px (mobile emulation), Albatross and Waterbuck: `.gantt .track` measures 0px wide; the 172px label column and the 60px busy column take the whole row, so no bar is visible.
- fix: Below 600px stack each gantt row: label and busy time on one line, the full-width track under it; drop the fixed label-column spacer from the tick axis.
- rule: WCAG 2.2 SC 1.4.10 Reflow
- status: fixed
- verification: 320px mobile emulation on :8484: every gantt track measures 258px on Albatross and Waterbuck (label and busy time on one line, track below); tick axis drops its fixed spacers. conductor.report-navigation requires tracks of at least 200px at 320px.

### F58 — Overflowing text is clipped at 320px instead of wrapping
- surface: requirement step text in the gantt disclosures, the Token spend table
- kind: a11y
- severity: medium
- evidence: Headless Chrome at 320px: `body{overflow-x:hidden}` reports scrollWidth 320, but `.step-text` spans end at x=342–384 on Albatross and the Token spend table ends at x=541 on Waterbuck; the overflow is cut off rather than reachable.
- fix: Let step text wrap (`min-width:0; overflow-wrap:anywhere`), remove the superseded token table, and stop hiding page overflow so a regression shows up as a measurable scroll instead of silent clipping.
- rule: WCAG 2.2 SC 1.4.10 Reflow
- status: fixed
- verification: body no longer hides horizontal overflow; with every disclosure opened at 320px, Albatross and Waterbuck report scrollWidth 320 = clientWidth and no element past the right edge; step text wraps (min-width:0, overflow-wrap:anywhere) and the token table is removed.

### F59 — Faint page text falls below 4.5:1 in both themes
- surface: plan page chrome outside the report sections: archived chip, harness badge "unknown", family version, launch-command tag, runbox "since", disclosure chevrons, axis tick labels and endpoints, requirement cells, Token spend headers and dim cells, footer
- kind: a11y
- severity: medium
- evidence: Headless Chrome on the :8484 preview of 2d1fa616, every visible text node measured against its painted background with details opened: `--text-faint` text reads 2.52–2.76:1 in the light theme (#a29b8d on #f7f4ef/#fff) and 2.40–2.81:1 in the dark theme (#565d64 on #0f1215/#1d2227); the success-green step check reads 3.58:1 and the done requirement cell 3.35:1, the active cell 4.32:1, in the light theme. The report sections pass.
- fix: Give faint text a token that clears 4.5:1 on every page surface (#736b5e light, #8a929a dark), draw requirement cell numbers in body text with the state carried by border, tint and a shape glyph, and use the scoped #047857 text green for the step check in the light theme.
- rule: WCAG 2.2 SC 1.4.3 Contrast (Minimum)
- status: fixed
- verification: Re-measured every visible text node on :8484 with details opened, light and dark: no failures on Albatross or Waterbuck (faint text #736b5e light 4.8:1 and up, #8a929a dark 5.1:1 and up; step check #047857; cells in body text). The only reported pair is the tick label measured against its own 1px tick mark, which it sits beside, not on.

### F60 — An all-unknown Delivery section repeats "No evidence recorded" for every stage
- surface: Delivery section on plans with no recorded delivery (Albatross, Gemsbok, Waterbuck)
- kind: ux
- severity: low
- evidence: Headless Chrome on the :8484 preview of 2d1fa616: Delivery is 1197px tall at 1280px on Albatross and 1416px at 320px, eight stage cards each reading "No evidence recorded. Unknown, not failed." beneath a summary that already says every stage is unknown, not failed. On Waterbuck it pushes Conductor activity further down the page.
- fix: When no stage has evidence, keep the summary line and fold the stage list with each stage's required proof into one disclosure; show the full list once any stage is recorded.
- rule: —
- status: fixed
- verification: With no stage recorded the stage list folds into "Stages and the proof each needs (8)"; Delivery measures 276px at 320px on Albatross and Waterbuck (1416px before). TestKrDeliveryStages and TestKrDeliveryUnknown pass.

## Automation editor pointer presses (killdeer navtest) — 2026-10-22

### F61 — Save or Cancel pressed straight after typing a name does nothing
- surface: /settings?section=automations&automation=<id> and the project Automations tab editor — Save automation, Cancel / Discard draft
- kind: ux
- severity: high
- evidence: Headless Chrome on 47b48274 with a 120 ms pointer press: type a new name, press Save. The press moves focus off the name field, its change handler calls renderAutomationTester(), which swaps the band table for a one-line "Evaluating…" (scroll anchoring holds Save in place), then the preview answers within a few milliseconds and the table re-expands without anchoring, so Save drops from y=666 to y=876. The release lands on a tester TD, the click goes to DIV.panel and the form never submits; 3 of 3 presses lost, Cancel the same. A 0 ms synthetic press (el.click()) never shows it. It is the "Save appears to do nothing" symptom of steps 2 and 4.
- fix: renderAutomationTester pins #automation-tester to its current height while it re-evaluates; evaluateAutomation releases the pin when the answer or the error lands.
- rule: — (a control must not move between pointer down and pointer up)
- status: fixed
- verification: 120 ms pointer presses in headless Chrome: Save of a rename and Cancel after typing both land; Save and Cancel measure y=666 before, on focus and after the answer (876 without the fix). conductor.automation-navigation asserts both presses and the positions, and fails 8 checks with the fix reverted.
