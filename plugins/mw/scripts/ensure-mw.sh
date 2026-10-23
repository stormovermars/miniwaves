#!/bin/bash
# SessionStart hook: make sure `mw` is installed and at least the plugin's version.
# Silent when it is; otherwise installs the matching GitHub release into ~/.local/bin.
# On Windows Claude Code runs hooks in Git Bash: uname -s says MINGW*/MSYS*/CYGWIN*, the
# asset is the .zip and the binary mw.exe.
set -u
want="0.3.6"
exe=""
case "$(uname -s)" in MINGW*|MSYS*|CYGWIN*) exe=".exe" ;; esac
mwbin="$HOME/.local/bin/mw$exe"; [ -x "$mwbin" ] || mwbin=$(command -v mw 2>/dev/null || true)
have=$("$mwbin" version 2>/dev/null | sed -n 's/^mw \([0-9.]*\) .*/\1/p')
if [ -n "$have" ] && [ "$(printf '%s\n%s\n' "$want" "$have" | sort -V | head -n1)" = "$want" ]; then
  exit 0
fi
case "$(uname -s)" in Darwin) os=Darwin ;; Linux) os=Linux ;; MINGW*|MSYS*|CYGWIN*) os=Windows ;; *) echo "mw: unsupported OS $(uname -s); see github.com/stormovermars/miniwaves" >&2; exit 0 ;; esac
case "$(uname -m)" in arm64|aarch64) arch=arm64 ;; x86_64) arch=x86_64 ;; *) echo "mw: unsupported CPU $(uname -m)" >&2; exit 0 ;; esac
if [ "$os" = Windows ]; then name="miniwaves_${os}_${arch}.zip"; else name="miniwaves_${os}_${arch}.tar.gz"; fi
base="https://github.com/stormovermars/miniwaves/releases/download/v${want}"
tmp=$(mktemp -d) || exit 0
trap 'rm -rf "$tmp"' EXIT
if ! curl -fsSL "$base/$name" -o "$tmp/$name" || ! curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt"; then
  echo "mw: download failed ($base); install manually" >&2; exit 0
fi
expected=$(awk -v f="$name" '$2 == f { print $1 }' "$tmp/checksums.txt")
if command -v sha256sum >/dev/null 2>&1; then actual=$(sha256sum "$tmp/$name" | awk '{print $1}'); else actual=$(shasum -a 256 "$tmp/$name" | awk '{print $1}'); fi
if [ -z "$expected" ] || [ "$expected" != "$actual" ]; then echo "mw: checksum mismatch, not installed" >&2; exit 0; fi
if [ "$os" = Windows ]; then
  if command -v unzip >/dev/null 2>&1; then unzip -q -o "$tmp/$name" mw.exe -d "$tmp" || exit 0
  else powershell.exe -NoProfile -Command "Expand-Archive -Force '$(cygpath -w "$tmp/$name" 2>/dev/null || echo "$tmp/$name")' '$(cygpath -w "$tmp" 2>/dev/null || echo "$tmp")'" >/dev/null || exit 0; fi
else
  tar -xzf "$tmp/$name" -C "$tmp" mw || exit 0
fi
mkdir -p "$HOME/.local/bin" && cp "$tmp/mw$exe" "$HOME/.local/bin/mw$exe" && chmod 755 "$HOME/.local/bin/mw$exe" && echo "mw ${want} installed to ~/.local/bin/mw$exe (add it to PATH if needed)"
exit 0
