#!/bin/sh
set -eu

dest="${MINIWAVES_INSTALL_DIR:-$HOME/.local/bin}"
binary="$dest/mw"
if [ -f "$binary" ]; then
  rm -f "$binary"
  echo "removed $binary"
else
  echo "miniwaves is not installed at $binary"
fi
