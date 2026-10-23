#!/bin/sh
# usage: scripts/check-mw-jobs-no-notify.sh
# TSK-064 guard: no non-test file of plugins/mw-jobs may call $.ui.notify (alerts are pane rows plus sound, never OS banners).
# The plugin test harness cannot read files, so this scan is the source-level half of the guard.
root="$(cd "$(dirname "$0")/.." && pwd)"
hits="$(grep -rnE '(\$\.)?ui[[:space:]]*\.[[:space:]]*notify[[:space:]]*\(' "$root/plugins/mw-jobs" \
  --include='*.ts' --include='*.tsx' --include='*.js' --include='*.mjs' --include='*.json' \
  --exclude='*.test.ts' --exclude='*.test.tsx' --exclude='*.d.ts' --exclude-dir=tests --exclude-dir=node_modules --exclude-dir=.claude-plugin || true)"
if [ -n "$hits" ]; then echo "FAIL: ui.notify is called:"; echo "$hits"; exit 1; fi
echo "ok: no ui.notify call in plugins/mw-jobs"
