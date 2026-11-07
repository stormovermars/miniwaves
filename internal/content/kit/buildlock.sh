#!/bin/sh
# Build lock for job "@@JOB@@": run one heavy build or test at a time.
# Usage: ./buildlock.sh COMMAND [ARGS...]   (exit code = the command's; 75 = timed out waiting)
# Env: BUILDLOCK_DIR (lock folder), BUILDLOCK_TIMEOUT (seconds to wait, default 1800).
# The lock is a folder; who may create it is decided by a noclobber file "$lock.own" (a shell builtin, O_EXCL: coreutils mkdir is not atomic everywhere, e.g. uutils on Ubuntu 26.04); a lock whose owner process is gone, or that has no pid file for over a minute (its owner died before writing it), is taken over (one waiter at a time, through a "$lock.takeover" file).
[ $# -gt 0 ] || { echo "usage: $0 COMMAND [ARGS...]" >&2; exit 2; }
lock="${BUILDLOCK_DIR:-${TMPDIR:-/tmp}/buildlock-@@JOB@@}"
timeout="${BUILDLOCK_TIMEOUT:-1800}"
waited=0
acquire() {
  ( set -C; : > "$lock.own" ) 2>/dev/null || return 1
  mkdir "$lock" 2>/dev/null || { rm -f "$lock.own"; return 1; }
}
while ! acquire; do
  owner=$(cat "$lock/pid" 2>/dev/null)
  # stale = owner process gone (kill -0, then ps: another user's live owner makes kill -0 fail with EPERM, which is not "dead"),
  # or no pid file and the folder is older than a minute (a live owner writes the pid within milliseconds of the mkdir)
  stale=0
  if [ -n "$owner" ]; then
    kill -0 "$owner" 2>/dev/null || ps -p "$owner" >/dev/null 2>&1 || stale=1
  elif [ -d "$lock" ] && [ -z "$(find "$lock" -maxdepth 0 -mmin -1 2>/dev/null)" ]; then
    stale=1
  fi
  if [ "$stale" = 1 ] && ( set -C; : > "$lock.takeover" ) 2>/dev/null; then
    # only the holder of $lock.takeover removes a stale lock, so the lock cannot be replaced under us;
    # re-read the owner first, another waiter may have finished the takeover already (a fresh lock has a new pid or is under a minute old)
    if [ "$(cat "$lock/pid" 2>/dev/null)" = "$owner" ] && { [ -n "$owner" ] || [ -z "$(find "$lock" -maxdepth 0 -mmin -1 2>/dev/null)" ]; }; then
      rm -rf "$lock" "$lock.own"
    fi
    rm -rf "$lock.takeover"
    continue
  fi
  # a takeover folder left by a crashed waiter is cleared after a minute
  [ -e "$lock.takeover" ] && find "$lock.takeover" -maxdepth 0 -mmin +1 -exec rm -rf {} \; 2>/dev/null
  [ -e "$lock.own" ] && [ ! -d "$lock" ] && find "$lock.own" -maxdepth 0 -mmin +1 -exec rm -f {} \; 2>/dev/null
  if [ "$waited" -ge "$timeout" ]; then
    echo "buildlock: timed out after ${timeout}s waiting for $lock (owner ${owner:-unknown})" >&2
    exit 75
  fi
  sleep 1
  waited=$((waited + 1))
done
echo $$ > "$lock/pid"
trap 'rm -rf "$lock" "$lock.own"' EXIT
trap 'exit 130' INT TERM HUP
"$@"
