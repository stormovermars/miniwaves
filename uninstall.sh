#!/bin/sh
set -eu

# mw and wr install together, so they uninstall together.
dest="${MINIWAVES_INSTALL_DIR:-$HOME/.local/bin}"
for b in mw wr; do
  if [ -f "$dest/$b" ]; then
    rm -f "$dest/$b"
    echo "removed $dest/$b"
  else
    echo "$b is not installed at $dest/$b"
  fi
done
