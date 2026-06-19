#!/usr/bin/env bash
# install.sh — wire fe.sh into your shell rc.
# Idempotent: safe to run more than once.
set -euo pipefail

# Resolve the absolute path to fe.sh sitting next to this script.
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)
FE_PATH="$SCRIPT_DIR/fe.sh"

if [[ ! -f "$FE_PATH" ]]; then
    echo "install: fe.sh not found at $FE_PATH" >&2
    exit 1
fi

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
