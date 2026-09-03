#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
out="${MINIWAVES_BUILD_DIR:-$root/build}"
action="${1:-build}"

case "$action" in
  build)
    mkdir -p "$out"
    (cd "$root" && go build -trimpath -o "$out/mw" .)
    echo "built $out/mw"
    ;;
  clean)
    rm -f "$out/mw" "$out/mw.exe"
    rmdir "$out" 2>/dev/null || true
    echo "cleaned $out"
    ;;
  *)
    echo "usage: ./build.sh [build|clean]" >&2
    exit 2
    ;;
esac
