#!/bin/sh
# usage: scripts/check-mw-jobs-no-home.sh
# TSK-033 guard: the mw-jobs plugin reads no HOME / USERPROFILE and hardcodes no ~/.claude (the plugin test harness has no HOME either).
root="$(cd "$(dirname "$0")/.." && pwd)"
hits="$(grep -rnE "env\.get\(['\"](HOME|USERPROFILE)['\"]|~/\.claude|\.claude/" "$root/plugins/mw-jobs/hooks" \
  --include='*.ts' --include='*.tsx' --exclude='*.test.ts' --exclude='*.test.tsx' || true)"
if [ -n "$hits" ]; then echo "FAIL:"; echo "$hits"; exit 1; fi
echo "ok: no HOME read and no ~/.claude in plugins/mw-jobs"
