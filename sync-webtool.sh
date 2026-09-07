#!/bin/bash
# Sync webtool from source of truth (this repo) to the myriplay project
# Usage: ./sync-webtool.sh

SRC="/home/travis/workspace/x85446/claudecodetricks/webtool"
DST="/home/travis/workspace/izuma/myriplay/.claude/webtool"

# Copy server and config files
cp "$SRC/serve.py" "$DST/serve.py"
cp "$SRC/requirements.txt" "$DST/requirements.txt"
cp "$SRC/vite.config.js" "$DST/vite.config.js"
cp "$SRC/package.json" "$DST/package.json"

# Copy static files
mkdir -p "$DST/static"
cp "$SRC/static/index.html" "$DST/static/index.html"
cp "$SRC/static/app.js" "$DST/static/app.js"
cp "$SRC/static/style.css" "$DST/static/style.css"

echo "Synced webtool to $DST"
