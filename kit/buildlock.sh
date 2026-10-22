#!/bin/sh
# Build lock for job "@@JOB@@": run one heavy build or test at a time.
# Usage: ./buildlock.sh COMMAND [ARGS...]   (exit code = the command's; 75 = timed out waiting)
# Env: BUILDLOCK_DIR (lock folder), BUILDLOCK_TIMEOUT (seconds to wait, default 1800).
# The lock is a folder made with mkdir, which is atomic; a lock whose owner process is gone is taken over.
[ $# -gt 0 ] || { echo "usage: $0 COMMAND [ARGS...]" >&2; exit 2; }
lock="${BUILDLOCK_DIR:-${TMPDIR:-/tmp}/buildlock-@@JOB@@}"
timeout="${BUILDLOCK_TIMEOUT:-1800}"
waited=0
while ! mkdir "$lock" 2>/dev/null; do
  owner=$(cat "$lock/pid" 2>/dev/null)
  if [ -n "$owner" ] && ! kill -0 "$owner" 2>/dev/null; then
    rm -rf "$lock"
    continue
  fi
  if [ "$waited" -ge "$timeout" ]; then
    echo "buildlock: timed out after ${timeout}s waiting for $lock (owner ${owner:-unknown})" >&2
    exit 75
  fi
  sleep 1
  waited=$((waited + 1))
done
echo $$ > "$lock/pid"
trap 'rm -rf "$lock"' EXIT
trap 'exit 130' INT TERM HUP
"$@"
