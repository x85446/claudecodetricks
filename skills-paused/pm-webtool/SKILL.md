---
name: pm-webtool
description: Launch the PM WebTool for reviewing and approving epics, features, requirements, and tests. Use when someone asks to review items, open the web UI, approve features, or launch the webtool.
argument-hint: (no arguments needed)
---

# PM WebTool — Human Review Web UI

Launch a web-based review tool with live reload. Vite serves the frontend on port 5179, FastAPI serves the API on port 5180.

## Invocation

```
/pm webtool
/pm-webtool
```

## Steps

1. **Locate the webtool.** Look for `.claude/webtool/serve.py` in the current project.
   If not found, tell the user to run the installer.

2. **Install Python dependencies.** Ensure pip exists, then install:
   ```bash
   python3 -m pip --version 2>/dev/null || python3 -m ensurepip --upgrade 2>/dev/null || sudo apt-get install -y python3-pip
   python3 -m pip install -q fastapi uvicorn
   ```
   Do NOT build alternative servers. Just install the dependencies.

3. **Install Node dependencies.** Vite for live reload:
   ```bash
   cd .claude/webtool && npm install 2>/dev/null || true
   ```

4. **Launch the API server** (background, port 5180):
   ```bash
   python3 .claude/webtool/serve.py --project "$(pwd)" --no-browser &
   ```

5. **Launch Vite dev server** (foreground, port 5179 with proxy to 5180):
   ```bash
   cd .claude/webtool && npx vite --config vite.config.js
   ```
   Vite auto-opens the browser at http://localhost:5179 with live reload.

6. **Confirm launch.** Tell the user:
   ```
   WebTool running:
     Frontend: http://localhost:5179 (Vite, live reload)
     API:      http://localhost:5180 (FastAPI)
     Database: .claude/db/marketing.sqlite
   ```

## What the WebTool Does

- **Browse** — tree view: Epics > Features > Requirements > Tests
- **Edit** — click any item to edit inline, version auto-bumps on save
- **Approve/Disapprove** — single click per item, bulk approve cascades to children
- **AI Feedback** — text or voice input per item, stored in database
- **Staleness** — yellow highlights on stale items, tooltips show why
- **Iterators** — glossary with clickable names, add/remove values inline
- **Voice** — mic icon on feedback fields, browser-native speech recognition
- **Live reload** — edit CSS/JS/HTML, browser updates instantly via Vite HMR

## Rules

1. WebTool is the ONLY place that sets `human_approved = 1`.
2. The PM skill and sub-skills never set `human_approved` directly.
3. Server reads/writes the same `.claude/db/marketing.sqlite` as all PM skills.
4. **Never reference `~/workspace/x85446/claudecodetricks/` from skills.** The webtool is deployed to `.claude/webtool/` in each project by the installer.
