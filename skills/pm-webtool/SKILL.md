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

1. **Check dependencies.** Verify fastapi and uvicorn are installed:
   ```bash
   python3 -c "import fastapi; import uvicorn" 2>/dev/null || python3 -m pip install fastapi uvicorn
   ```

2. **Locate the webtool.** The serve.py file is at:
   ```
   ~/workspace/x85446/claudecodetricks/webtool/serve.py
   ```

3. **Launch the server.** Run in the background:
   ```bash
   python3 ~/workspace/x85446/claudecodetricks/webtool/serve.py --project "$(pwd)" &
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
