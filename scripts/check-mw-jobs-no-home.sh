#!/bin/sh
# usage: scripts/check-mw-jobs-no-home.sh
# Guard: the mw-jobs plugin hooks stay silent and never block; its commands/hooks hardcode no ~/.claude and the hook script always exits 0.
root="$(cd "$(dirname "$0")/.." && pwd)"
hits="$(grep -rnE "~/\.claude|\.claude/" "$root/plugins/mw-jobs/hooks" "$root/plugins/mw-jobs/commands" || true)"
if [ -n "$hits" ]; then echo "FAIL:"; echo "$hits"; exit 1; fi
if grep -nE 'notify|osascript|exit [1-9]' "$root/plugins/mw-jobs/hooks/refresh.sh"; then echo "FAIL: refresh.sh must not notify or fail"; exit 1; fi
echo "ok: mw-jobs hooks are silent, non-blocking and use no ~/.claude"
