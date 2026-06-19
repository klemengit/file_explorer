#!/usr/bin/env bash
# install.sh — wire fe.sh into your shell rc.
#
# Works two ways:
#   • From a clone:   ./install.sh                 (uses the fe.sh next to it)
#   • Piped remotely: wget -qO- <raw>/install.sh | bash
#                     curl -fsSL <raw>/install.sh | bash
#                     (downloads fe.sh into ~/.local/share/fe/)
#
# Idempotent: safe to run more than once.
set -euo pipefail

FE_REPO_RAW="${FE_REPO_RAW:-https://raw.githubusercontent.com/klemengit/file_explorer/main}"
INSTALL_DIR="${FE_INSTALL_DIR:-$HOME/.local/share/fe}"

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" 2>/dev/null && pwd || echo "")

_fetch() {  # _fetch URL DEST
    if command -v curl &>/dev/null; then
        curl -fsSL "$1" -o "$2"
    elif command -v wget &>/dev/null; then
        wget -qO "$2" "$1"
    else
        echo "install: need curl or wget to download fe.sh" >&2
        return 1
    fi
}

# ── dependency check ──────────────────────────────────────────────────────────
missing=()
for dep in fzf gum; do
    command -v "$dep" &>/dev/null || missing+=("$dep")
done
if (( ${#missing[@]} )); then
    echo "install: missing required dependencies: ${missing[*]}" >&2
    echo "  fzf: https://github.com/junegunn/fzf" >&2
    echo "  gum: https://github.com/charmbracelet/gum" >&2
    exit 1
fi

echo "Optional tools (features degrade gracefully if absent):"
for opt in nvim fd xdg-open zip unzip; do
    if command -v "$opt" &>/dev/null; then
        echo "  ✓ $opt"
    else
        echo "  · $opt (not found)"
    fi
done
echo

# ── locate fe.sh (local clone) or download it ─────────────────────────────────
if [[ -n "$SCRIPT_DIR" && -f "$SCRIPT_DIR/fe.sh" ]]; then
    FE_PATH="$SCRIPT_DIR/fe.sh"
    echo "Using local fe.sh: $FE_PATH"
else
    mkdir -p "$INSTALL_DIR"
    FE_PATH="$INSTALL_DIR/fe.sh"
    echo "Downloading fe.sh → $FE_PATH"
    _fetch "$FE_REPO_RAW/fe.sh" "$FE_PATH" \
        || { echo "install: failed to download $FE_REPO_RAW/fe.sh" >&2; exit 1; }
fi
echo

# ── pick the shell rc ─────────────────────────────────────────────────────────
shell_name=$(basename "${SHELL:-bash}")
case "$shell_name" in
    zsh)  rc="$HOME/.zshrc" ;;
    bash) rc="$HOME/.bashrc" ;;
    *)    rc="$HOME/.bashrc"
          echo "install: unrecognized shell '$shell_name', defaulting to $rc" >&2 ;;
esac

# ── append the source line (idempotently) ─────────────────────────────────────
source_line="source \"$FE_PATH\""
if [[ -f "$rc" ]] && grep -qsF "$FE_PATH" "$rc"; then
    echo "Already installed in $rc — nothing to do."
else
    printf '\n# fe — terminal file explorer\n%s\n' "$source_line" >> "$rc"
    echo "Added to $rc:"
    echo "    $source_line"
fi

echo
echo "Done. Reload your shell to start using it:"
echo "    source $rc"
echo "Then run:  fe"
