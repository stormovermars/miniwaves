#!/bin/bash
# Claude Code status line (clickable via OSC 8): `mw status --line --link` (reads the status JSON from stdin for the cwd). Silent, always exits 0.
exe=""
case "$(uname -s)" in MINGW*|MSYS*|CYGWIN*) exe=".exe" ;; esac
mwbin="$HOME/.local/bin/mw$exe"; [ -x "$mwbin" ] || mwbin=$(command -v mw 2>/dev/null || true)
[ -n "$mwbin" ] || exit 0
"$mwbin" status --line --link 2>/dev/null || true
exit 0
