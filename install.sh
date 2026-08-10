#!/usr/bin/env bash
# install.sh — download a prebuilt fe binary and put it on your PATH.
#
# Needs nothing but curl (or wget). To build from source instead — your own
# edits, or an architecture the releases don't cover — use build.sh.
#
#   wget -qO- <raw>/install.sh | bash
#   curl -fsSL <raw>/install.sh | bash
#   ./install.sh                        (from a clone; same thing)
#
# Environment:
#   FE_VERSION   install a specific tag (default: the latest release)
#   FE_BIN_DIR   where to put the binary (default: ~/.local/bin)
#
# Idempotent: safe to run more than once — it overwrites in place.
set -euo pipefail

FE_RELEASES="${FE_RELEASES:-https://github.com/klemengit/file_explorer/releases}"
BIN_DIR="${FE_BIN_DIR:-$HOME/.local/bin}"
FE_VERSION="${FE_VERSION:-}"

tmpdir=""
cleanup() { [[ -n "$tmpdir" ]] && rm -rf "$tmpdir"; return 0; }  # never fail under set -e
trap cleanup EXIT

# ── what are we on? ───────────────────────────────────────────────────────────
os=$(uname -s)
if [[ "$os" != "Linux" ]]; then
    echo "install: fe is a Linux program — there are no $os builds." >&2
    echo "  It leans on lsblk, udisksctl, /proc and xdg-open throughout." >&2
    exit 1
fi

case "$(uname -m)" in
    x86_64|amd64)  arch=amd64 ;;
    aarch64|arm64) arch=arm64 ;;
    *) echo "install: no prebuilt binary for $(uname -m)." >&2
       echo "  Build it yourself with build.sh (needs Go)." >&2
       exit 1 ;;
esac
asset="fe-linux-$arch"

# ── fetch ─────────────────────────────────────────────────────────────────────
if command -v curl &>/dev/null; then
    _fetch() { curl -fsSL "$1" -o "$2"; }
elif command -v wget &>/dev/null; then
    _fetch() { wget -qO "$2" "$1"; }
else
    echo "install: need curl or wget to download the binary." >&2
    exit 1
fi

if [[ -n "$FE_VERSION" ]]; then
    base="$FE_RELEASES/download/$FE_VERSION"
    echo "Installing fe $FE_VERSION ($arch)"
else
    base="$FE_RELEASES/latest/download"   # GitHub redirects this to the newest tag
    echo "Installing the latest fe ($arch)"
fi

tmpdir=$(mktemp -d "${TMPDIR:-/tmp}/fe-install.XXXXXX")
if ! _fetch "$base/$asset" "$tmpdir/fe"; then
    echo "install: could not download $base/$asset" >&2
    echo "  Check that the release exists, or build from source with build.sh." >&2
    exit 1
fi

# ── verify ────────────────────────────────────────────────────────────────────
# A tampered or truncated download is worth catching before it lands on PATH.
if command -v sha256sum &>/dev/null && _fetch "$base/SHA256SUMS" "$tmpdir/SHA256SUMS" 2>/dev/null; then
    want=$(awk -v a="$asset" '$2 == a || $2 == "*"a {print $1}' "$tmpdir/SHA256SUMS")
    got=$(sha256sum "$tmpdir/fe" | cut -d' ' -f1)
    if [[ -z "$want" ]]; then
        echo "Note: SHA256SUMS lists no entry for $asset — skipping verification."
    elif [[ "$want" != "$got" ]]; then
        echo "install: checksum mismatch — refusing to install." >&2
        echo "  expected $want" >&2
        echo "  got      $got" >&2
        exit 1
    else
        echo "Checksum OK"
    fi
else
    echo "Note: no SHA256SUMS available — skipping verification."
fi

# ── install ───────────────────────────────────────────────────────────────────
mkdir -p "$BIN_DIR"
install -m755 "$tmpdir/fe" "$BIN_DIR/fe"
echo "Installed: $BIN_DIR/fe  ($("$BIN_DIR/fe" --version 2>/dev/null || echo "version unknown"))"

# ── after-install notes ───────────────────────────────────────────────────────
case ":$PATH:" in
    *":$BIN_DIR:"*) ;;
    *) echo
       echo "Note: $BIN_DIR is not on your PATH. Add this to your shell rc:"
       echo "    export PATH=\"$BIN_DIR:\$PATH\"" ;;
esac

# The pre-Go version was a sourced shell function; that line now points at a
# file this repo no longer ships, and would shadow the binary if it survives.
# Commented-out lines are ignored — they are already harmless.
old_line='^[[:space:]]*[^#[:space:]].*fe\.sh'
for rc in "$HOME/.bashrc" "$HOME/.zshrc"; do
    if [[ -f "$rc" ]] && grep -qsE "$old_line" "$rc"; then
        echo
        echo "Note: $rc still sources the old shell version:"
        grep -nE "$old_line" "$rc" | sed 's/^/    /'
        echo "  Delete that line — it overrides the binary you just installed."
    fi
done

echo
echo "Done. Run:  fe"
