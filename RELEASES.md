# Releases

What changed, for people who use this.

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
