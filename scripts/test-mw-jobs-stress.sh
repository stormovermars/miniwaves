#!/bin/sh
# usage: scripts/test-mw-jobs-stress.sh
# Runs the mw-jobs stress test. Its big fixtures live in testdata/mw-jobs-stress/ so they do not ship in the
# installed plugin; a plugin test may only import from inside the plugin folder, so this copies the plugin and
# the stress files to a temp folder and runs `claude plugin test` there.
set -eu
root="$(cd "$(dirname "$0")/.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
cp -R "$root/plugins/mw-jobs/." "$tmp/"
mkdir -p "$tmp/hooks/fixtures/stress"
for f in "$root"/testdata/mw-jobs-stress/*; do
  case "$(basename "$f")" in
    stress.test.tsx) cp "$f" "$tmp/hooks/stress.test.tsx" ;;
    README.md) ;;
    *) cp "$f" "$tmp/hooks/fixtures/stress/" ;;
  esac
done
claude plugin test "$tmp" 2>&1 | grep -v '^@@' | tail -40
