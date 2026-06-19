#!/usr/bin/env bash
# fe — CLI file explorer
# Requires: gum, fzf  (optional: fd, nvim, xdg-open, zip/unzip)
# Source from your shell rc: source /path/to/fe.sh
#
# Command mode (default — letters are commands, not search):
#   h ← parent   j ↓   k ↑   l → enter/open
#   enter open · O open-with · e nvim
#   y yank · x cut · p paste · d delete · r rename · z zip/unzip
#   s filter (type to narrow) · f deep find · q quit
# Press s to filter the current dir; esc returns to command mode.

_FE_SH_PATH="${BASH_SOURCE[0]}"
_FE_CLIP="${XDG_RUNTIME_DIR:-/tmp}/.fe_clip"
_FE_MARKS="${XDG_DATA_HOME:-$HOME/.local/share}/fe/bookmarks"

# ── Tokyo Night palette ───────────────────────────────────────────────────────
_FE_B='\033[38;2;122;162;247m'
_FE_G='\033[38;2;158;206;106m'
_FE_O='\033[38;2;224;175;104m'
_FE_R='\033[38;2;247;118;142m'
_FE_M='\033[38;2;86;95;137m'
_FE_BOLD='\033[1m'
_FE_RST='\033[0m'

# ── fzf theme ─────────────────────────────────────────────────────────────────
_FE_FZF_OPTS=(
    --color "fg:#c0caf5,bg:-1,hl:#e0af68"
    --color "fg+:#c0caf5,bg+:#283457,hl+:#e0af68"
    --color "prompt:#7aa2f7,pointer:#7aa2f7,marker:#9ece6a,gutter:-1"
    --color "info:#565f89,header:#7aa2f7,border:#565f89"
    --layout=reverse
    --border=rounded
    --prompt="  "
    --pointer="▶"
    --marker="✓"
)

# Keys that are commands in command mode but must type literally while filtering.
_FE_KEYS="h,j,k,l,O,e,y,x,p,d,r,z,s,f,m,b,?,q"

# ── main ──────────────────────────────────────────────────────────────────────
fe() {
    if ! command -v gum &>/dev/null; then
        echo "fe: gum not found — https://github.com/charmbracelet/gum" >&2
        return 1
    fi

    local dir
    dir=$(realpath "${1:-$PWD}") || return 1

    local state help
    state=$(mktemp "${TMPDIR:-/tmp}/.fe_state.XXXXXX") || return 1
    help=$(mktemp "${TMPDIR:-/tmp}/.fe_help.XXXXXX")  || return 1
    cat > "$help" <<'FEHELP'

  fe — keybindings

  Navigate     h parent  ·  j / k down · up  ·  l / enter open
  Open         enter open (default app)  ·  O open with…  ·  e nvim
  Clipboard    y yank (copy)  ·  x cut  ·  p paste here
  Manage       d delete  ·  r rename  ·  z zip / unzip
  Search       s filter (type; esc exits)  ·  f deep find
  Bookmarks    m bookmark this dir  ·  b jump (ctrl-d deletes)
  Help / quit  ? toggle help  ·  q quit

FEHELP

    # Command-mode key bindings. Each action writes a verb to $state then accepts,
    # so the loop below knows what was pressed alongside the highlighted item.
    # s flips fzf into search mode (letters type); esc flips back.
    local binds=(
        "j:down"
        "k:up"
        "h:execute-silent(printf parent > '$state')+accept"
        "l:execute-silent(printf into > '$state')+accept"
        "enter:execute-silent(printf open > '$state')+accept"
        "O:execute-silent(printf openwith > '$state')+accept"
        "e:execute-silent(printf nvim > '$state')+accept"
        "y:execute-silent(printf yank > '$state')+accept"
        "x:execute-silent(printf cut > '$state')+accept"
        "p:execute-silent(printf paste > '$state')+accept"
        "d:execute-silent(printf delete > '$state')+accept"
        "r:execute-silent(printf rename > '$state')+accept"
        "z:execute-silent(printf zip > '$state')+accept"
        "f:execute-silent(printf find > '$state')+accept"
        "m:execute-silent(printf mark > '$state')+accept"
        "b:execute-silent(printf jump > '$state')+accept"
        "q:execute-silent(printf quit > '$state')+accept"
        "?:toggle-preview"
        "s:enable-search+change-prompt(/ )+unbind($_FE_KEYS)"
        "esc:disable-search+clear-query+change-prompt(  )+rebind($_FE_KEYS)"
    )
    local bind_args=() b
    for b in "${binds[@]}"; do bind_args+=(--bind "$b"); done

    local hint="  hjkl move · enter open · s filter · f find · ? help · q quit  "

    while true; do
        local list
        list=$(
            echo ".."
            if [[ -f "$_FE_CLIP" ]]; then
                printf '[paste %s: %s]\n' \
                    "$(cut -d: -f1 "$_FE_CLIP")" \
                    "$(basename "$(cut -d: -f2- "$_FE_CLIP")")"
            fi
            _fe_ls "$dir"
        )

        : > "$state"
        local pick action clean
        pick=$(printf '%s\n' "$list" | fzf \
            "${_FE_FZF_OPTS[@]}" \
            "${bind_args[@]}" \
            --disabled \
            --ansi \
            --height=20 \
            --header="  ${dir}" \
            --preview="cat '$help'" \
            --preview-window="down,45%,wrap,border-top,hidden" \
            --border-label="$hint" \
            --border-label-pos="0:bottom" \
            --color "label:#565f89" \
        ) || { rm -f "$state" "$help"; return 0; }

        action=$(cat "$state")
        clean=$(printf '%s' "$pick" | sed 's/\x1b\[[0-9;]*[mK]//g')

        case "$action" in
            quit)
                rm -f "$state" "$help"; return 0
                ;;
            parent)
                local parent; parent=$(dirname "$dir")
                [[ "$parent" != "$dir" ]] && { dir="$parent"; cd "$dir"; }
                continue
                ;;
            find)
                local found
                found=$(_fe_fzf "$dir") || continue
                [[ -z "$found" ]] && continue
                if [[ -d "$found" ]]; then dir="$found"; else dir=$(dirname "$found"); fi
                cd "$dir"
                continue
                ;;
            mark)
                mkdir -p "$(dirname "$_FE_MARKS")"
                if grep -qxF "$dir" "$_FE_MARKS" 2>/dev/null; then
                    gum log --level info "Already bookmarked: $dir"
                else
                    echo "$dir" >> "$_FE_MARKS"
                    gum log --level info "Bookmarked: $dir"
                fi
                continue
                ;;
            jump)
                local marked
                marked=$(_fe_bookmarks) || continue
                [[ -z "$marked" ]] && continue
                if [[ -d "$marked" ]]; then
                    dir="$marked"; cd "$dir"
                else
                    gum log --level error "No longer exists: $marked"
                fi
                continue
                ;;
        esac

        [[ -z "$clean" ]] && continue

        # special rows
        case "$clean" in
            "..")
                case "$action" in
                    into|open|"")
                        local parent; parent=$(dirname "$dir")
                        [[ "$parent" != "$dir" ]] && { dir="$parent"; cd "$dir"; }
                        ;;
                esac
                continue
                ;;
            \[paste*)
                case "$action" in
                    into|open|paste|"")
                        local clip_action
                        clip_action=$(gum choose "paste here" "dismiss") || continue
                        case "$clip_action" in
                            "paste here") _fe_paste "$dir" ;;
                            "dismiss")    rm -f "$_FE_CLIP" ;;
                        esac
                        ;;
                esac
                continue
                ;;
        esac

        local name="${clean%/}"
        local target="$dir/$name"

        case "$action" in
            into|open|"")
                # enter / l: dirs navigate in, files open in the default app
                if [[ -d "$target" ]]; then
                    dir="$target"; cd "$dir"
                elif [[ -e "$target" ]]; then
                    xdg-open "$target" &>/dev/null &
                fi
                ;;
            *)
                _fe_do "$action" "$target" "$dir"
                ;;
        esac
    done
}

# ── directory listing ─────────────────────────────────────────────────────────
_fe_ls() {
    local dir="$1"
    find "$dir" -maxdepth 1 -mindepth 1 -type d -printf '%f/\n' 2>/dev/null \
        | sort -f \
        | while IFS= read -r d; do printf '\033[1;34m%s\033[0m\n' "$d"; done
    find "$dir" -maxdepth 1 -mindepth 1 -type l -printf '%f\n' 2>/dev/null \
        | sort -f \
        | while IFS= read -r f; do printf '\033[36m%s\033[0m\n' "$f"; done
    find "$dir" -maxdepth 1 -mindepth 1 -type f -printf '%f\n' 2>/dev/null \
        | sort -f
}

# ── file action ───────────────────────────────────────────────────────────────
_fe_do() {
    local action="$1" file="$2" dir="$3"
    local name
    name=$(basename "$file")

    case "$action" in
        openwith)
            local cmd
            cmd=$(gum input --placeholder="command…" --prompt="  ") || return 0
            [[ -n "$cmd" ]] && $cmd "$file" &
            ;;
        nvim) nvim "$file" ;;
        yank) echo "copy:$file" > "$_FE_CLIP"
              gum log --level info "Yanked: $name" ;;
        cut)  echo "cut:$file" > "$_FE_CLIP"
              gum log --level warn "Cut: $name" ;;
        paste) _fe_paste "$dir" ;;
        delete)
            gum confirm --prompt.foreground="#f7768e" "Delete '$name'?" \
            && { rm -rf "$file"; gum log --level warn "Deleted: $name"; }
            ;;
        rename)
            local new
            new=$(gum input --value="$name" --placeholder="new name…" --prompt="  ") || return 0
            [[ -n "$new" && "$new" != "$name" ]] && mv "$file" "$dir/$new"
            ;;
        zip)
            if [[ "$file" == *.zip ]]; then
                unzip -q "$file" -d "${file%.zip}"
                gum log --level info "Unzipped: ${name%.zip}/"
            else
                zip -r "$file.zip" "$file"
                gum log --level info "Zipped: $name.zip"
            fi
            ;;
    esac
}

# ── deep search ───────────────────────────────────────────────────────────────
_fe_fzf() {
    local dir="$1"
    local finder
    command -v fd &>/dev/null \
        && finder=(fd --hidden --follow . "$dir") \
        || finder=(find "$dir" -not -path "*/.*")

    local sel
    sel=$("${finder[@]}" 2>/dev/null \
        | sed "s|^$dir/||" \
        | fzf "${_FE_FZF_OPTS[@]}" \
              --height=40% \
              --header="  $dir" \
    ) || return 1

    [[ -n "$sel" ]] && echo "$dir/$sel"
}

# ── bookmarks ─────────────────────────────────────────────────────────────────
_fe_bookmarks() {
    local marks="$_FE_MARKS"
    if [[ ! -s "$marks" ]]; then
        gum log --level warn "No bookmarks yet — press m to add one"
        return 1
    fi
    local sel
    sel=$(cat "$marks" \
        | fzf "${_FE_FZF_OPTS[@]}" \
              --height=40% \
              --header="  bookmarks  ·  enter: go  ·  ctrl-d: delete" \
              --bind "ctrl-d:execute-silent(grep -vxF {} '$marks' > '$marks.tmp'; mv '$marks.tmp' '$marks')+reload(cat '$marks')" \
    ) || return 1
    [[ -n "$sel" ]] && echo "$sel"
}

# ── paste ─────────────────────────────────────────────────────────────────────
_fe_paste() {
    local dest="$1"
    if [[ ! -f "$_FE_CLIP" ]]; then
        gum log --level error "Nothing to paste"; return 1
    fi
    local clip_mode clip_src clip_name
    clip_mode=$(cut -d: -f1 "$_FE_CLIP")
    clip_src=$(cut -d: -f2- "$_FE_CLIP")
    clip_name=$(basename "$clip_src")
    local target="$dest/$clip_name"

    if [[ -e "$target" ]]; then
        gum confirm --prompt.foreground="#e0af68" "Overwrite '$clip_name'?" || return 0
    fi

    if [[ "$clip_mode" == "copy" ]]; then
        cp -r "$clip_src" "$target"
        gum log --level info "Copied: $clip_name"
    else
        mv "$clip_src" "$target"
        rm -f "$_FE_CLIP"
        gum log --level info "Moved: $clip_name"
    fi
}
