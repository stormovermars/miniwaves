#!/bin/sh
set -eu

repo="${MINIWAVES_REPO:-stormovermars/miniwaves}"
dest="${MINIWAVES_INSTALL_DIR:-$HOME/.local/bin}"
version="${MINIWAVES_VERSION:-latest}"

case "$(uname -s)" in
  Darwin) os=Darwin ;;
  Linux) os=Linux ;;
  *) echo "use install.ps1 on Windows" >&2; exit 1 ;;
esac
case "$(uname -m)" in
  arm64) arch=arm64 ;;
  aarch64) arch=arm64 ;;
  x86_64) arch=x86_64 ;;
  *) echo "unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
if [ "$version" = latest ]; then
  url="https://github.com/$repo/releases/latest/download/miniwaves_${os}_${arch}.tar.gz"
else
  url="https://github.com/$repo/releases/download/$version/miniwaves_${os}_${arch}.tar.gz"
fi

mkdir -p "$dest"
archive="$tmp/miniwaves.tar.gz"
curl -fsSL "$url" -o "$archive"
checksums_url="${url%/*}/checksums.txt"
curl -fsSL "$checksums_url" -o "$tmp/checksums.txt"
expected="$(awk -v file="$(basename "$url")" '$2 == file { print $1 }' "$tmp/checksums.txt")"
[ -n "$expected" ] || { echo "release checksum not found" >&2; exit 1; }
if command -v sha256sum >/dev/null 2>&1; then
  actual="$(sha256sum "$archive" | awk '{print $1}')"
else
  actual="$(shasum -a 256 "$archive" | awk '{print $1}')"
fi
[ "$actual" = "$expected" ] || { echo "release checksum mismatch" >&2; exit 1; }
tar -xzf "$archive" -C "$tmp"
install -m 0755 "$tmp/mw" "$dest/mw"
echo "installed miniwaves to $dest/mw"
case ":$PATH:" in *":$dest:"*) ;; *) echo "add $dest to PATH: export PATH=\"$dest:\$PATH\"" ;; esac

# Claude Code plugins: best effort, never fails the binary install. MW_NO_PLUGINS=1 skips.
if [ "${MW_NO_PLUGINS:-}" = 1 ]; then
  echo "plugins: skipped (MW_NO_PLUGINS=1)"
elif ! command -v claude >/dev/null 2>&1; then
  echo "plugins: claude not found; later run: mw update --plugins"
else
  claude plugin marketplace add "$repo" >/dev/null 2>&1 || true
  if ! mo="$(claude plugin marketplace update miniwaves 2>&1)"; then
    echo "plugins: failed: marketplace update: $(printf '%s' "$mo" | tail -n 1)"
  else
    listing="$(claude plugin list 2>/dev/null || true)"
    if printf '%s\n' "$listing" | grep -q "[[:space:]]tidewatch@miniwaves\$"; then
      if claude plugin uninstall tidewatch@miniwaves >/dev/null 2>&1; then
        echo "plugin tidewatch: removed (renamed to mw-watch)"
      fi
    fi
    for p in mw mw-watch; do
      if printf '%s\n' "$listing" | grep -q "[[:space:]]$p@miniwaves\$"; then
        verb=updated; cmd=update
      else
        verb=installed; cmd=install
      fi
      if po="$(claude plugin "$cmd" "$p@miniwaves" 2>&1)"; then
        echo "plugin $p: $verb"
      else
        echo "plugin $p: failed: $(printf '%s' "$po" | tail -n 1)"
      fi
    done
  fi
fi
