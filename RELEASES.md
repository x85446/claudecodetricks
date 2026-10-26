# Releases

What changed, for people who use this.

## 2026-10-26 — Plans you can recognize at a glance

**Improved**
- Every place that lists your plans shows each one as the first letter of its name, in the order the plans will run: capital for a plan split into parallel teams, lower case otherwise, colored by state.
- Blocked plans stay red even while a session is looking at them.
- Projects of any size list every plan; the 26-plan limit is gone.

## 2026-10-26 — Shared execution ownership and independent scheduling

**New**
- CLI iterate and the scheduler share execution ownership, preventing two executors from starting the same project together.
- The Conductor runs as an independent service, with scheduling controls separate from plan graphs and reports.

**Improved**
- Named plans respect unfinished prerequisites, and every execution view uses stable dependency order.
- Product and iterate planning group related work into coherent plans while preserving requirements, acceptance checks and completed history.

## 2026-10-23 — Plans that wait their turn

**New**
- Plans name the plans they build on, and the conductor runs each one only after those finish. Plans with nothing unfinished behind them run in any order, so you can stage a whole release at once: `/ip stage M1-04..M1-10`.
- See the order at a glance: `/ip graph` in the terminal, or the new **Graph** tab on the dashboard, where waiting plans are drawn with dashed arrows and the Plans tab marks them **Waiting**.
- `/ip retrofit` gives plans you already have their dependency line, reading their milestones and notes.

**Fixed**
- When a plan stops on something only you can supply, the plans behind it wait instead of launching and stopping one after another. The dashboard names the one plan holding them up.

## 2026-10-22 — SSH config edits that stay clean

**Fixed**
- Editing your SSH sites with `/ssh-config` keeps its backups out of the folder ssh reads, so an old backup can't quietly bring back a port forward you removed.

**Improved**
- `/ssh-config` tells you when a backup or editor copy in `~/.ssh/config.d` is being loaded as live config, and `list` and `sort` point out hosts defined twice.

## 2026-10-22 — Plan reports you can trust, and simpler automations

**New**
- Every plan page explains what happened: the outcome, which requirements are done and the proof behind each, where the time went and why, who held the plan and what it waited on, retries, what it cost, and how far delivery got. Anything nobody recorded says "unknown" instead of showing a misleading zero.
- Choose which projects use each automation by dragging them together, or with the keyboard.
- Try an automation's rules before saving: pick Green, Yellow or Red and see which row would launch.
- Codex runs count toward a plan's usage alongside Claude Code.

**Improved**
- Plan pages lead with the outcome and progress, link each requirement to its proof, and keep the activity log in its own scrolling box.
- Automation names match everywhere they appear, and unsaved changes stay marked until you save or discard them.
- What a whole session spent is shown apart from what the plan itself used.

**Fixed**
- Named Automations always opens the list, even with an editor open, and Save returns you to it.
- Buttons work on the first click, including right after you type a name or a priority and while the dashboard refreshes.
- Completed plans no longer report zero done, and long quiet stretches are explained or marked unknown rather than flagged as failures.

## 2026-10-21 — Reusable automations and clearer settings

**New**
- Name automation rules once in Settings and attach them to several projects. Each project has its own enable switch and priority. Used by shows shared scope; Duplicate creates independent rules, and referenced rules are protected from deletion.
- Set green/red percentages at each elapsed hour from 0 through 5. Try the power bands uses only your draft, elapsed time and usage percentage, with a clear green indicator at zero.
- Weekdays and Weekends come with editable full-day hours, displayed as 00:00–23:59.

**Improved**
- Completed CLI repairs hand unblocked plans back to Conductor. Running badges identify CLI versus Conductor ownership, while parked failures remain visible.
- Plan identity stays above a compact, scrollable activity log. Directory aliases share one project entry.
- Subscription setup shows the AI tool, installed program, account/usage status and power band first. Detect and Verify identify the host executable; advanced options use readable argument rows, with inline help for technical choices.
- Dashboard and subscription switches make activation visible. Disabled settings remain readable and easy to re-enable.
- Overdrive testers keep the green bar and show “✓ Overdrive achieved” at the target. Provider hard limits still apply.

## 2026-10-16 — One place to schedule and follow AI work

**New**
- Open `http://localhost:8420` to browse projects, queued plans, complete timelines and live AI activity in one site.
- Choose named launch sequences in priority order, the budget bands each permits, and recurring hours. Run continuously, process one plan per window, or follow a subscription’s overdrive windows.
- Manage AI Subscriptions, shared named power bands and named time windows in Settings. Overdrive targets express percentage used: 100% uses available allowance before exhaustion; 60% stops the override at 60% used.
- Preview scheduling and budget decisions with read-only simulators. Compact rows, consistent actions and draft dialogs keep navigation and edits predictable.

**Improved**
- The binary owns scheduling, provider streams, exclusive plan ownership and checkpoint recovery. Claude and Codex skills control that same configuration.
- Window endings can finish the current checkpoint, finish the claimed plan while other gates permit, or interrupt the owned process. Fresh allowance data and hard provider limits remain mandatory.
- Installation reuses one managed webserver on port 8420, preserves rollback files and refuses to displace an unrelated service.

## 2026-10-13 — The test runner works on Windows

**New**
- The test runner builds and runs on Windows as well as macOS and Linux. Stopping a test, or letting it time out, stops everything it started. Shell tests run in Git Bash when it is installed, so the same test command works on all three systems.

## 2026-10-05 — Tidy tutorial menus, and runs that settle their own design questions

**Improved**
- Tutorial menus and title boxes are fully closed and fit their contents at any terminal width. Long lines wrap inside the box instead of spilling past it.
- Each tutorial shows its time on one line, `~45 min running (10 hands-on)`, so you can tell keyboard time from waiting time. Durations line up in one column, and on a narrow screen they move under the title instead of breaking.
- Tutorial colours follow a written standard, switch off for pipes, `NO_COLOR` and dumb terminals, and can be forced on with `FORCE_COLOR`.
- An unattended run no longer stops to ask whether to extend an API its feature needs, or to report a bug it found in its own work. It extends the interface, updates everything that uses it, fixes what it broke, and keeps going. It stops for you only when it needs a secret, physical access, or someone outside the project.

## 2026-10-05 — Plans that run themselves overnight

**New**
- Approve a plan and walk away. Stage it (`/ip stage <name>`), enroll the project once (`iterate-run nightly enroll`), and the nightly tick starts it inside the project's allowed hours while you are off the keyboard — then resumes it on later ticks until it finishes. Nothing you have not approved is ever started unattended.
- The status line tells you more at a glance: orange for a plan approved to run overnight, dark green for a run that has stalled, bold green for one that is really working, `⏰` for a project enrolled in the nightly tick — and the next plan's letter is always there, dimmed, so an empty segment can only mean the status line itself is broken.
- `iterate-run status` lists every plan with its state, and `iterate-run name peek` previews the next plan's name without using it up.
- The dashboard shows approved plans as `queued`, and a project driven by the nightly tick reads `tick launchd`.

**Improved**
- Unattended runs no longer stop to ask permission over machine time. A slow test that the plan's own change invalidated now runs during the night instead of waiting for you to come back and start it.
- New plans are written as one straightforward list by default; say "team this" when you want parallel teams.

**Fixed**
- A project whose last plan had been archived showed no plan segment at all in the status line.
- The unattended runner could pick the right plan and then fail to start it; it now starts it.

## 2026-09-10 — Always-on dashboard, and plan names that don't run out

**New**
- Your iterate dashboard can now run by itself. One command installs it, it starts when you log in, and it restarts if it ever dies. It lives at http://localhost:8420.
- A browser window can stay parked on the dashboard permanently, reopening itself if you close it. It runs in its own profile, so your everyday Chrome — bookmarks, extensions, signed-in accounts — is untouched.
- `make run` shows you the dashboard immediately, on a free port, without installing anything.
- The dashboard now tells you which assistant produced each plan, and gives you the right command to run it for that assistant. Plans made before this was recorded say so rather than guessing.
- Each project now shows what its unattended runner is doing — working, watching, or stood down — including the case where a runner is switched on but has nothing to trigger it, so one that will never actually fire can no longer look healthy.

**Improved**
- Plan names no longer run out. The pool grew from 110 animals to 1,442, with every letter carrying at least 13.
- You can check the dashboard is up from the command line instead of opening a browser.

**Fixed**
- Naming a plan used to fail outright once a letter was used up, which eventually blocked creating plans at all. It now moves to the next available letter, and only complains when every name really is taken — telling you how many remain under each letter.
