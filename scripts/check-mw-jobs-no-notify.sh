#!/bin/sh
# usage: scripts/check-mw-jobs-no-notify.sh
# Guard: mw-jobs has no pane code any more: no .ts/.tsx files and no $.ui / $.tool registration.
root="$(cd "$(dirname "$0")/.." && pwd)"
hits="$(find "$root/plugins/mw-jobs" \( -name '*.ts' -o -name '*.tsx' \) | head; grep -rnE '\$\.(ui|tool|command)\.' "$root/plugins/mw-jobs" || true)"
if [ -n "$hits" ]; then echo "FAIL:"; echo "$hits"; exit 1; fi
echo "ok: no pane code in plugins/mw-jobs"
