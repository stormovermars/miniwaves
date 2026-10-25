#!/bin/bash
# SessionStart / PostToolUse(Bash running `mw `) hook: quietly regenerate jobs.md and status.md for the
# board of the current folder. Never blocks, never prints, ignores every failure.
input=$(cat 2>/dev/null || true)
case "$input" in
  *'"tool_name"'*)  # PostToolUse: only commands that run mw
    case "$input" in *'"command":"mw '*|*'"command": "mw '*|*'mw board'*|*'mw news'*|*' mw '*|*'&&mw '*) ;; *) exit 0 ;; esac ;;
esac
exe=""
case "$(uname -s)" in MINGW*|MSYS*|CYGWIN*) exe=".exe" ;; esac
mwbin="$HOME/.local/bin/mw$exe"; [ -x "$mwbin" ] || mwbin=$(command -v mw 2>/dev/null || true)
[ -n "$mwbin" ] || exit 0
cd "${CLAUDE_PROJECT_DIR:-.}" 2>/dev/null || exit 0
"$mwbin" board --md >/dev/null 2>&1 || true
"$mwbin" news >/dev/null 2>&1 || true
exit 0
