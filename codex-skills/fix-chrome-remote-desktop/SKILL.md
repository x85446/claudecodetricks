---
name: "fix-chrome-remote-desktop"
description: "Use when the user says \"fixup chrome remote desktop\", \"CRD is offline\", \"CRD lights up then greys out\", \"chrome remote desktop stopped working\", \"reconnect chrome remote desktop\", or invokes $fix-chrome-remote-desktop."
---


## Machine-Specific Facts (cypressMini)

- **User:** `travis` (UID `503`)
- **Google account paired with the CRD host:** `travismccollum@gmail.com`
- **Verify at:** https://remotedesktop.google.com/access (must be signed in as `travismccollum@gmail.com`)
- **Host binary:** `/Library/PrivilegedHelperTools/ChromeRemoteDesktopHost.app/Contents/MacOS/remoting_me2me_host`
- **Host config:** `/Library/PrivilegedHelperTools/org.chromium.chromoting.json`
- **Enable flag:** `/Library/PrivilegedHelperTools/org.chromium.chromoting.me2me_enabled` (presence = enabled)
- **LaunchAgent plist:** `/Library/LaunchAgents/org.chromium.chromoting.plist` (loads into `gui/503`)
- **Broker LaunchDaemon plist:** `/Library/LaunchDaemons/org.chromium.chromoting.broker.plist` (loads into `system`)

The host has TCC ScreenCapture already granted (`com.google.chromeremotedesktop.me2me-host`), so **no System Settings clicks are required.** All fixes are SSH-safe from a remote session.

## Symptom → Cause Cheatsheet

| Symptom | Likely cause |
|---|---|
| cypressMini shows offline at remotedesktop.google.com/access | Host process not running |
| cypressMini lights up green for ~1s then goes grey | Host started, failed to reach broker, sent `SendHostOfflineReason: INITIALIZATION_FAILED` to Google |
| `pgrep -f remoting_me2me_host` returns nothing | Host process not running |
| `/tmp/crd_host.log` shows `Cannot connect to IPC through server name chromoting.agent_process_broker_mojo_ipc` | Broker LaunchDaemon not bootstrapped in `system` domain |
| `launchctl print-disabled gui/503 \| grep chromoting` shows `disabled` | LaunchAgent was explicitly disabled |
| `/Library/PrivilegedHelperTools/org.chromium.chromoting.me2me_enabled.backup` exists (instead of `.me2me_enabled`) | Enable flag file was renamed to soft-disable |

## Try the keepalive daemon first

Since 2026-07-30 there's a boot-time LaunchDaemon that runs the whole fix idempotently. Before doing anything by hand, kick it and see if that's enough:

```bash
sudo launchctl kickstart -k system/com.travis.crd-keepalive 2>&1
sleep 4
sudo tail -20 /var/log/crd-keepalive.log
pgrep -lf remoting_me2me_host
```

If the log ends with `host up (pid=...)` and `pgrep` shows the two processes, you're done — verify at https://remotedesktop.google.com/access and stop here.

Only if kicking the daemon doesn't restore the host, fall through to the manual diagnostic sequence below.

**Daemon file locations** (canonical copies in this skill dir; deployed via `install-daemon.sh`):
- Script: `/usr/local/bin/crd-fixup.sh`
- Plist: `/Library/LaunchDaemons/com.travis.crd-keepalive.plist`
- Log: `/var/log/crd-keepalive.log`

**To uninstall the daemon:**
```bash
sudo launchctl bootout system/com.travis.crd-keepalive
sudo rm /Library/LaunchDaemons/com.travis.crd-keepalive.plist /usr/local/bin/crd-fixup.sh
```

## Steps

### 1. Diagnose (read-only)

Run these to figure out which of the three soft-disables are in play:

```bash
# Is the process running at all?
pgrep -lf remoting_me2me_host || echo "(host NOT running)"

## What this skill does

<!-- codex-port: moved out of the startup description, which is charged against Codex's manifest budget in every session. This text is documentation, not routing signal, so it belongs at the body level where it loads on trigger. No trigger phrase was moved. -->

Diagnoses and restores the CRD host on cypressMini (macOS) when it's failed to register with Google, so cypressMini shows as online at https://remotedesktop.google.com/access.

# Is the enable-flag file in the right shape?
ls -la /Library/PrivilegedHelperTools/org.chromium.chromoting.me2me_enabled*

# Is the LaunchAgent disabled in gui/503?
sudo launchctl print-disabled gui/503 | grep chromoting

# Is the broker enabled in system?
sudo launchctl print-disabled system | grep 'chromoting.broker'

# Is the broker currently loaded?
sudo launchctl print system/org.chromium.chromoting.broker 2>&1 | grep -E 'state|active count' || echo "(broker NOT in system domain)"
```

### 2. Restore the enable flag (if `.backup` exists)

```bash
if [ -f /Library/PrivilegedHelperTools/org.chromium.chromoting.me2me_enabled.backup ]; then
    sudo mv /Library/PrivilegedHelperTools/org.chromium.chromoting.me2me_enabled.backup \
            /Library/PrivilegedHelperTools/org.chromium.chromoting.me2me_enabled
fi
```

### 3. Bootstrap the broker into the system domain (order matters)

The host worker connects to `chromoting.agent_process_broker_mojo_ipc` — a Mach service the broker LaunchDaemon publishes. If the broker isn't registered, the host will send `INITIALIZATION_FAILED` to Google (the "lights up then greys out" symptom).

```bash
sudo launchctl enable system/org.chromium.chromoting.broker
sudo launchctl bootstrap system /Library/LaunchDaemons/org.chromium.chromoting.broker.plist
```

Ignore "service already loaded" — that's fine. Verify:

```bash
sudo launchctl print system/org.chromium.chromoting.broker 2>&1 | grep -E 'state|active count'
```

Expected: `state = not running`, `active count = 0` — the broker is on-demand, launched by launchd when the host connects to its Mach service.

### 4. Try to load the LaunchAgent (preferred, persistent path)

```bash
sudo launchctl enable gui/503/org.chromium.chromoting
sudo launchctl bootstrap gui/503 /Library/LaunchAgents/org.chromium.chromoting.plist
sleep 3
pgrep -lf remoting_me2me_host
```

If `pgrep` shows two processes (a supervisor + a worker) — you're done. Skip to Step 6.

### 5. Fallback: run the host directly via `nohup` (if launchctl bootstrap keeps failing)

`launchctl bootstrap gui/503 …` sometimes fails with `Bootstrap failed: 5: Input/output error` in a stale launchd state. The direct-run works and survives SSH disconnect via `nohup` + `disown`:

```bash
sudo -u travis nohup \
  /Library/PrivilegedHelperTools/ChromeRemoteDesktopHost.app/Contents/MacOS/remoting_me2me_host \
  --host-config=/Library/PrivilegedHelperTools/org.chromium.chromoting.json \
  > /tmp/crd_host.log 2>&1 &
disown
sleep 3
pgrep -lf remoting_me2me_host
tail -20 /tmp/crd_host.log
```

Look for `Host ready to receive connections.` in the log — that's the success marker.

**Caveat:** the nohup'd process does NOT persist across reboot. A reboot usually clears whatever was jamming `launchctl bootstrap`, so on the next boot the LaunchAgent should come up on its own. If it doesn't, you'll need to SSH in and re-run Steps 3–5.

### 6. Verify end-to-end

Tell the user to open **https://remotedesktop.google.com/access** signed in as `travismccollum@gmail.com`. cypressMini should show green/online within ~30 seconds. If it lights up then greys out again, go back to Step 3 — the broker isn't registered.

## Notes / Gotchas

- **Don't run the host binary as root.** Always `sudo -u travis` — the host keys and OAuth tokens in the config are per-user; running as root breaks the pairing.
- **Chrome Remote Desktop is not just uninstalled when it "looks" uninstalled.** `/Applications/Chrome Remote Desktop Host.app` may not exist, but the actual binaries live in `/Library/PrivilegedHelperTools/ChromeRemoteDesktopHost.app/`. Don't reinstall unless the binaries are truly gone.
- **If the host DID get uninstalled** (no `ChromeRemoteDesktopHost.app` in `/Library/PrivilegedHelperTools/`), the fix is to reinstall. Google's macOS installer URL is `https://dl.google.com/edgedl/chrome-remote-desktop/chromeremotedesktophost.dmg` (test with `curl -I` — the URL changes occasionally). After install, re-pair by visiting `https://remotedesktop.google.com/access` → **Set up remote access**, grab the auth code, run `start-host --code=… --redirect-url=… --name=cypressMini`.
- **TCC screen-recording permission for `com.google.chromeremotedesktop.me2me-host` is already granted** on cypressMini (verified 2026-07-29). If someone runs `sudo tccutil reset ScreenCapture`, that grant disappears and can only be restored by a physical click in System Settings → Privacy & Security → Screen Recording. Don't reset TCC on this Mac casually.
- **Two soft-disables exist independently** — you need to clear both:
  1. Filesystem: `me2me_enabled` renamed to `.backup`
  2. launchd: `launchctl disable gui/503/org.chromium.chromoting`
  Fixing only one leaves the host disabled.
- **Related — Remote Management / VNC on this Mac is a dead end without physical access:** `com.apple.screensharing.agent` has TCC ScreenCapture = 0 (denied) and can only be flipped by a click in System Settings. Don't waste time enabling Remote Management via `kickstart` as a workaround; it'll accept VNC connections but return "Screen Sharing is not permitted."

## Output

Report back to the user:
- Which soft-disables were found and fixed (which of enable-flag / launchctl-disable / broker-not-bootstrapped)
- Whether the LaunchAgent loaded cleanly (persistent) or we fell back to nohup (won't survive reboot)
- Confirmation URL: https://remotedesktop.google.com/access — ask them to verify cypressMini shows online
- Any remaining state changes made during troubleshooting that should be reverted (e.g. if Remote Management was turned on as a fallback attempt, offer to turn it back off)
