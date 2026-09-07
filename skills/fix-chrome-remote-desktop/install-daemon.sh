#!/bin/bash
# install-daemon.sh — install (or reinstall) the CRD keepalive LaunchDaemon.
# Run as: sudo ./install-daemon.sh
set -e

HERE="$(cd "$(dirname "$0")" && pwd)"

sudo install -m 755 -o root -g wheel "$HERE/crd-fixup.sh" /usr/local/bin/crd-fixup.sh
sudo install -m 644 -o root -g wheel "$HERE/com.travis.crd-keepalive.plist" /Library/LaunchDaemons/com.travis.crd-keepalive.plist

# Reload the daemon so plist/script changes take effect immediately.
sudo launchctl bootout system/com.travis.crd-keepalive 2>/dev/null || true
sudo launchctl bootstrap system /Library/LaunchDaemons/com.travis.crd-keepalive.plist

echo "Installed. Log: /var/log/crd-keepalive.log"
echo "Kick anytime with: sudo launchctl kickstart -k system/com.travis.crd-keepalive"
