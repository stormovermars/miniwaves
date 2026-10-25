#!/bin/bash
# PostToolUse(Bash) hook: after an mw verb that writes the board, quietly regenerate jobs.md (mw board --md) for
# the board of the hook's cwd. It never touches the news files (/mw-news does), never prints, always exits 0.
input=$(cat 2>/dev/null || true)
cmd=$(printf '%s' "$input" | grep -o '"command" *: *"[^"]*"' | head -1 | sed 's/^"command" *: *"//; s/"$//')
verb=""; seen=""; skip=""
for w in $cmd; do
  if [ -n "$skip" ]; then skip=""; continue; fi
  if [ -z "$seen" ]; then
    case "${w##*/}" in mw|mw.exe) seen=1 ;; esac
    continue
  fi
  case "$w" in
    -t|-s|--tenant|--session) skip=1 ;;
    -*) ;;
    *) verb="$w"; break ;;
  esac
done
case "$verb" in
  add|ready|start|done|block|unblock|ask|answer|accept|reject|wave|note|edit) ;;
  *) exit 0 ;;
esac
cwd=$(printf '%s' "$input" | grep -o '"cwd" *: *"[^"]*"' | head -1 | sed 's/^"cwd" *: *"//; s/"$//; s/\\\\/\\/g')
[ -n "$cwd" ] && cd "$cwd" 2>/dev/null || exit 0
exe=""
case "$(uname -s)" in MINGW*|MSYS*|CYGWIN*) exe=".exe" ;; esac
mwbin="$HOME/.local/bin/mw$exe"; [ -x "$mwbin" ] || mwbin=$(command -v mw 2>/dev/null || true)
[ -n "$mwbin" ] || exit 0
"$mwbin" board --md >/dev/null 2>&1 || true
exit 0
