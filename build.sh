#!/usr/bin/env bash
# build.sh — build fe from source and put the binary on your PATH.
#
# Most people want install.sh instead, which downloads a prebuilt binary and
# needs no Go. Use this one to build your own edits, or on an architecture the
# releases don't cover.
#
# Works two ways:
#   • From a clone:   ./build.sh                 (builds the fe-tui next to it)
#   • Piped remotely: wget -qO- <raw>/build.sh | bash
#                     curl -fsSL <raw>/build.sh | bash
#                     (clones the repo into a temp dir, builds, throws it away)
#
# Idempotent: safe to run more than once — it just rebuilds and overwrites.
set -euo pipefail

FE_REPO="${FE_REPO:-https://github.com/klemengit/file_explorer.git}"
BIN_DIR="${FE_BIN_DIR:-$HOME/.local/bin}"
GO_MIN=1.24.2

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" 2>/dev/null && pwd || echo "")
tmpdir=""
cleanup() { [[ -n "$tmpdir" ]] && rm -rf "$tmpdir"; return 0; }  # never fail under set -e
trap cleanup EXIT

# ── dependency check ──────────────────────────────────────────────────────────
if ! command -v go &>/dev/null; then
    echo "build: fe is a Go program and needs the Go toolchain." >&2
    echo "  https://go.dev/dl/  (or your package manager: golang / go)" >&2
    exit 1
fi

go_ver=$(go env GOVERSION 2>/dev/null | sed 's/^go//')
if [[ -n "$go_ver" ]] && [[ "$(printf '%s\n%s\n' "$GO_MIN" "$go_ver" | sort -V | head -1)" != "$GO_MIN" ]]; then
    echo "build: Go $GO_MIN or newer is required (found $go_ver)." >&2
    exit 1
fi
echo "Using Go ${go_ver:-unknown}"

echo
echo "Optional tools (features degrade gracefully if absent):"
for opt in nvim xdg-open zip unzip lsblk udisksctl zoxide; do
    if command -v "$opt" &>/dev/null; then
        echo "  ✓ $opt"
    else
        echo "  · $opt (not found)"
    fi
done
echo

# ── locate the source (local clone) or fetch it ───────────────────────────────
if [[ -n "$SCRIPT_DIR" && -f "$SCRIPT_DIR/fe-tui/main.go" ]]; then
    SRC_DIR="$SCRIPT_DIR/fe-tui"
    echo "Building from local source: $SRC_DIR"
else
    command -v git &>/dev/null || {
        echo "build: need git to fetch the source when run outside a clone" >&2
        exit 1
    }
    tmpdir=$(mktemp -d "${TMPDIR:-/tmp}/fe-build.XXXXXX")
    echo "Cloning $FE_REPO"
    git clone --depth 1 --quiet "$FE_REPO" "$tmpdir/src" \
        || { echo "build: failed to clone $FE_REPO" >&2; exit 1; }
    SRC_DIR="$tmpdir/src/fe-tui"
fi

# ── build and install ─────────────────────────────────────────────────────────
build_out=$(mktemp "${TMPDIR:-/tmp}/fe-build.XXXXXX")
trap 'cleanup; rm -f "$build_out"' EXIT

# Stamp the binary so `fe --version` says something useful. A checkout at a tag
# gives "v0.1.0"; anything else gives "v0.1.0-3-gabc1234" or just the commit.
ver=$(cd "$SRC_DIR" && git describe --tags --always --dirty 2>/dev/null || echo "dev")

echo "Compiling…"
(cd "$SRC_DIR" && go build -ldflags "-X main.version=$ver" -o "$build_out" .) \
    || { echo "build: failed" >&2; exit 1; }

mkdir -p "$BIN_DIR"
install -m755 "$build_out" "$BIN_DIR/fe"
echo "Installed: $BIN_DIR/fe"

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
