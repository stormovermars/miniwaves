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

# fetch REPO ASSET VERSION DIR: download a release asset, check it against the release's
# checksums.txt and unpack it into DIR. Returns non-zero on any failure.
fetch() {
  if [ "$3" = latest ]; then
    u="https://github.com/$1/releases/latest/download/$2"
  else
    u="https://github.com/$1/releases/download/$3/$2"
  fi
  mkdir -p "$4" || return 1
  curl -fsSL "$u" -o "$4/$2" || return 1
  curl -fsSL "${u%/*}/checksums.txt" -o "$4/checksums.txt" || return 1
  expected="$(awk -v file="$2" '$2 == file || $2 == "*" file { print $1 }' "$4/checksums.txt")"
  [ -n "$expected" ] || { echo "release checksum not found for $2" >&2; return 1; }
  if command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "$4/$2" | awk '{print $1}')"
  else
    actual="$(shasum -a 256 "$4/$2" | awk '{print $1}')"
  fi
  [ "$actual" = "$expected" ] || { echo "release checksum mismatch for $2" >&2; return 1; }
  tar -xzf "$4/$2" -C "$4"
}

mkdir -p "$dest"
if [ -n "${MW_INSTALL_FILE:-}" ]; then
  # Local artefact (CI smoke tests): a binary or a .tar.gz holding mw. No download, no checksum.
  [ -f "$MW_INSTALL_FILE" ] || { echo "MW_INSTALL_FILE not found: $MW_INSTALL_FILE" >&2; exit 1; }
  echo "MW_INSTALL_FILE set: skipping download and checksum verification" >&2
  case "$MW_INSTALL_FILE" in
    *.tar.gz|*.tgz) tar -xzf "$MW_INSTALL_FILE" -C "$tmp" ;;
    *) cp "$MW_INSTALL_FILE" "$tmp/mw" ;;
  esac
else
  fetch "$repo" "miniwaves_${os}_${arch}.tar.gz" "$version" "$tmp" || exit 1
fi
install -m 0755 "$tmp/mw" "$dest/mw"
echo "installed miniwaves to $dest/mw"

# wr (wave rider) installs with mw, always its latest release. Best effort: a failure is a note.
wr_repo="${WR_REPO:-stormovermars/wr}"
if fetch "$wr_repo" "wr_${os}_${arch}.tar.gz" latest "$tmp/wr" && install -m 0755 "$tmp/wr/wr" "$dest/wr"; then
  echo "installed wr to $dest/wr"
else
  echo "wr: not installed (download failed); retry: mw update"
fi
case ":$PATH:" in *":$dest:"*) ;; *) echo "add $dest to PATH: export PATH=\"$dest:\$PATH\"" ;; esac

# Fanfare defaults (on, set all) only where the user has not chosen; MW_NO_FANFARE=1 or MW_NO_PLUGINS=1 skips.
if [ "${MW_NO_FANFARE:-}" != 1 ] && [ "${MW_NO_PLUGINS:-}" != 1 ]; then
  "$dest/mw" user fanfare-defaults 2>/dev/null || true
fi

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
    ok=1
    for p in mw mw-jobs; do
      if printf '%s\n' "$listing" | grep -q "[[:space:]]$p@miniwaves\$"; then
        verb=updated; cmd=update
      else
        verb=installed; cmd=install
      fi
      if po="$(claude plugin "$cmd" "$p@miniwaves" 2>&1)"; then
        echo "plugin $p: $verb"
      else
        ok=0
        echo "plugin $p: failed: $(printf '%s' "$po" | tail -n 1)"
      fi
    done
    # Retired pane plugins go only after the new ones are in place (claude honours CLAUDE_CONFIG_DIR itself).
    if [ "$ok" = 1 ]; then
      for p in tidewatch mw-watch; do
        if printf '%s\n' "$listing" | grep -q "[[:space:]]$p@miniwaves\$"; then
          if claude plugin uninstall "$p@miniwaves" >/dev/null 2>&1; then
            echo "plugin $p: removed (replaced by mw-jobs)"
          fi
        fi
      done
    fi
  fi
fi
