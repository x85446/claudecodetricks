# metro-recovery — deep Metro cache recovery

Run when a plain `npx react-native start --reset-cache` doesn't unblock Metro: stuck bundling, stale modules after a dep change, red screen referencing a path that no longer exists, or module-resolution errors that don't match current code.

**Pre-check**: have you already tried `pkill -f "react-native start" && npx react-native start --reset-cache`? If not, do that first — this skill is the escalation, not the first resort.

## Sequence (run top-to-bottom; report which step unblocks)

```bash
# 1. Kill everything Metro-adjacent
pkill -f "react-native start" || true
pkill -f metro || true
lsof -ti:8081 | xargs kill -9 2>/dev/null || true

# 2. Clear Watchman + Haste-map + Metro caches
watchman watch-del-all
rm -rf "$TMPDIR/metro-"* "$TMPDIR/haste-map-"* 2>/dev/null

# 3. Reinstall deps ONLY if package-lock.json changed
#    (skip if lockfile is unchanged — node_modules wipe is slow)
if git diff --quiet HEAD -- package-lock.json 2>/dev/null; then
  echo "package-lock.json unchanged — skipping node_modules wipe"
else
  rm -rf node_modules && npm install   # repo is npm-only; never yarn
fi

# 4. iOS only — refresh Pods
[ -d ios ] && (cd ios && pod install && cd ..)

# 5. Restart Metro with full cache reset
npx react-native start --reset-cache &
```

## Reporting back

When the issue clears, report which step did it. Patterns observed:

- **Step 2 alone fixes it** → Watchman state was stale (common after switching branches)
- **Step 3 needed** → a dep version actually changed and node_modules was inconsistent
- **Step 4 needed** → Podfile or native module updated; iOS-side bundle was stale
- **All five and still broken** → escalate to Bryan with: the exact red-screen error, output of `npx react-native doctor`, and `git status`

## Don't use this skill when

- The error is a TypeScript / lint error (Metro is fine — fix the type error)
- App boots but a specific feature is broken (this is a Metro-cache skill, not a runtime debugger — go to `docs/claude/debugging.md`)
- Build error from `xcodebuild` or `gradlew assembleDebug` (those are build-tool failures, not Metro — go to `docs/claude/android.md` or `docs/claude/devices.md`)
- iOS Pods error specifically (`pod install` fails its own validation — read the error, don't blanket-rerun)

## Related leaves

- `docs/claude/debugging.md` — tool priority order (CDP → Metro → adb logcat → IDE)
- `docs/claude/gotchas.md` — known false positives (port 8081 already-in-use noise, etc.)
- `docs/claude/android.md` — Android build commands and JAVA_HOME
- `docs/claude/devices.md` — iOS build commands and idb
