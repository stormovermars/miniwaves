#!/bin/sh
# usage: scripts/check-release.sh releases/<version>.md [board.json]
# Validates front-matter keys and that every item id exists in the board.
f="${1:?usage: check-release.sh release.md [board.json]}"
board="${2:-.miniwaves.json}"
bad=0
problem() { echo "$f: $1"; bad=1; }
[ -f "$f" ] || { echo "$f: not found"; exit 1; }
[ -f "$board" ] || { echo "$board: not found"; exit 1; }
fm=$(tr -d '\r' < "$f" | awk 'NR==1&&$0!="---"{exit} NR>1&&$0=="---"{exit} NR>1{print}')
[ -n "$fm" ] || { echo "$f: missing front matter"; exit 1; }
for k in version date build tested items; do
  printf '%s\n' "$fm" | grep -q "^$k:[[:space:]]*[^[:space:]]" || problem "missing key: $k"
done
# optional: evidence (when present it must not be empty)
printf '%s\n' "$fm" | grep -q "^evidence:" && ! printf '%s\n' "$fm" | grep -q "^evidence:[[:space:]]*[^[:space:]]" && problem "empty key: evidence"
items=$(printf '%s\n' "$fm" | sed -n 's/^items:[[:space:]]*//p' | tr -d '[]' | tr ',' ' ')
for id in $items; do
  grep -q "\"$id\"" "$board" || problem "item not in board: $id"
done
exit $bad
