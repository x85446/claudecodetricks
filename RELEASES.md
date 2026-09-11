# Releases

What changed, for people who use this.

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
