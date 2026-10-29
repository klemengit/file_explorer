#!/usr/bin/env bash
# fe-chooser.sh — run fe as the desktop file dialog, through
# xdg-desktop-portal-termfilechooser. See "fe as your file dialog" in README.md.
#
# The portal calls:  fe-chooser.sh MULTIPLE DIRECTORY SAVE PATH OUT DEBUG [TERMCMD...]
#   MULTIPLE, DIRECTORY, SAVE  "1" or "0"
#   PATH     the folder to start in; for a save, the suggested file's full path
#   OUT      file to write the chosen paths into, one per line (empty = cancel)
#   TERMCMD  the terminal to run fe in. The portal always passes one, and
#            its own default is kitty, so set it in the portal config:
#            env=TERMCMD=xdg-terminal-exec --app-id=fe-chooser --title=fe --
#
# Environment:
#   FE_BIN   the fe binary (default: fe on PATH, else ~/.local/bin/fe)
set -euo pipefail

multiple=$1 directory=$2 save=$3 path=$4 out=$5
[[ "${6:-0}" == 1 ]] && set -x
shift 6 || shift $#

# The portal runs as a user service, whose PATH may not include ~/.local/bin.
fe=${FE_BIN:-$(command -v fe || echo "$HOME/.local/bin/fe")}

args=(--choose "$out")
[[ $multiple == 1 ]] && args+=(--multiple)
[[ $directory == 1 ]] && args+=(--directory)
if [[ $save == 1 ]]; then
    args+=(--save "$(basename "$path")")
    dir=$(dirname "$path")
else
    dir=$path
fi
[[ -d $dir ]] || dir=$HOME

(($#)) || set -- xdg-terminal-exec --app-id=fe-chooser --title=fe --
exec "$@" "$fe" "${args[@]}" "$dir"
