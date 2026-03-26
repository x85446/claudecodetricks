---
name: pm-webtool
description: Launch the PM WebTool for reviewing and approving epics, features, requirements, and tests. Use when someone asks to review items, open the web UI, approve features, or launch the webtool.
argument-hint: (no arguments needed)
---

# PM WebTool — Human Review Web UI

Launch a web-based review tool for browsing, editing, and approving PM database items. Product is auto-detected from the database.

## Invocation

```
/pm webtool
/pm-webtool
```

## Steps

1. **Locate the webtool.** Look for `serve.py` in this order:
   ```
   .claude/webtool/serve.py          (project-local install)
   ```
   If not found, the installer hasn't run. Tell the user:
   ```
   WebTool not installed. Run the skill installer to deploy it:
     ~/workspace/x85446/claudecodetricks/skills/skillinstall.sh pm
   ```

2. **Check dependencies.** Verify the server can run:
   ```bash
   python3 -c "import fastapi; import uvicorn" 2>/dev/null || python3 -m pip install fastapi uvicorn
   ```
   If Python is unavailable, try Node.js — the webtool includes a `serve-node.js` fallback. (TODO: not yet implemented)

3. **Launch the server.** Run in the background:
   ```bash
   python3 .claude/webtool/serve.py --project "$(pwd)" &
   ```
   This starts on port 8420 and auto-opens the browser.

4. **Confirm launch.** Tell the user:
   ```
   WebTool running at http://localhost:8420
   Database: .claude/db/marketing.sqlite
   Press Ctrl+C in the terminal to stop.
   ```

## What the WebTool Does

- **Browse** — tree view: Epics > Features > Requirements > Tests
- **Edit** — click any item to edit inline, version auto-bumps on save
- **Approve/Disapprove** — single click per item, bulk approve cascades to children
- **AI Feedback** — text or voice input per item, stored in database
- **Staleness** — yellow highlights on stale items, tooltips show why
- **Iterators** — glossary with clickable names, add/remove values inline
- **Voice** — mic icon on feedback fields, browser-native speech recognition

## Rules

1. WebTool is the ONLY place that sets `human_approved = 1`.
2. The PM skill and sub-skills never set `human_approved` directly.
3. Server reads/writes the same `.claude/db/marketing.sqlite` as all PM skills.
4. **Never reference `~/workspace/x85446/claudecodetricks/` from skills.** The webtool is deployed to `.claude/webtool/` in each project by the installer.
