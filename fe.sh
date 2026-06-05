#!/usr/bin/env bash
# fe — CLI file explorer
# Requires: gum, fzf
# Source from your shell rc: source /path/to/fe.sh

_FE_SH_PATH="${BASH_SOURCE[0]}"
_FE_CLIP="${XDG_RUNTIME_DIR:-/tmp}/.fe_clip"

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

# ── main ──────────────────────────────────────────────────────────────────────
fe() {
    if ! command -v gum &>/dev/null; then
        echo "fe: gum not found — https://github.com/charmbracelet/gum" >&2
        return 1
    fi

    local dir
    dir=$(realpath "${1:-$PWD}") || return 1

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

        local output key pick clean
        output=$(printf '%s\n' "$list" | fzf \
            "${_FE_FZF_OPTS[@]}" \
            --header="  ${dir}" \
            --height=20 \
            --ansi \
            --expect="right,ctrl-l,ctrl-f" \
            --border-label="  ↑↓ navigate  ·  enter: open  ·  ctrl-l: actions  ·  ctrl-f: search  " \
            --border-label-pos="0:bottom" \
            --color "label:#565f89" \
        ) || return 0

        key=$(printf '%s' "$output" | head -1)
        pick=$(printf '%s' "$output" | sed -n '2p')
        [[ -z "$pick" ]] && return 0
        clean=$(printf '%s' "$pick" | sed 's/\x1b\[[0-9;]*[mK]//g')

        # ctrl-f → deep search
        if [[ "$key" == "ctrl-f" ]]; then
            local found
            found=$(_fe_fzf "$dir") || continue
            [[ -z "$found" ]] && continue
            if [[ -d "$found" ]]; then
                dir="$found"
            else
                dir=$(dirname "$found")
            fi
            cd "$dir"
            continue
        fi

        case "$clean" in
            "..")
                local parent
                parent=$(dirname "$dir")
                [[ "$parent" != "$dir" ]] && { dir="$parent"; cd "$dir"; }
                continue
                ;;
            \[paste*)
                _fe_paste "$dir"
                continue
                ;;
        esac

        local name="${clean%/}"
        local target="$dir/$name"

        # right arrow → action menu (works on both files and dirs)
        if [[ "$key" == "right" || "$key" == "ctrl-l" ]]; then
            _fe_action "$target" "$dir"
            continue
        fi

        # Enter: dirs navigate, files open in nvim
        if [[ -d "$target" ]]; then
            dir="$target"; cd "$dir"
        elif [[ -e "$target" ]]; then
            nvim "$target"
        fi
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

# ── action menu ───────────────────────────────────────────────────────────────
_fe_action() {
    local file="$1" dir="$2"
    local name
    name=$(basename "$file")

    local items=(
        "o  open"
        "e  nvim"
        "O  open with…"
        "y  yank (copy)"
        "x  cut"
        "p  paste here"
        "d  delete"
        "r  rename"
        "z  zip / unzip"
    )

    local output key
    output=$(printf '%s\n' "${items[@]}" | fzf \
        "${_FE_FZF_OPTS[@]}" \
        --header="  $name" \
        --height=13 \
        --no-sort \
        --expect="o,e,O,y,x,p,d,r,z" \
    ) || return 0

    key=$(printf '%s' "$output" | head -1)
    [[ -z "$key" ]] && key=$(printf '%s' "$output" | sed -n '2p' | cut -c1)
    [[ -z "$key" ]] && return 0

    case "$key" in
        o) xdg-open "$file" &>/dev/null & ;;
        e) nvim "$file" ;;
        O)
            local cmd
            cmd=$(gum input --placeholder="command…" --prompt="  ") || return 0
            [[ -n "$cmd" ]] && $cmd "$file" &
            ;;
        y) echo "copy:$file" > "$_FE_CLIP"
           gum log --level info "Yanked: $name" ;;
        x) echo "cut:$file" > "$_FE_CLIP"
           gum log --level warn "Cut: $name" ;;
        p) _fe_paste "$dir" ;;
        d)
            gum confirm --prompt.foreground="#f7768e" "Delete '$name'?" \
            && { rm -rf "$file"; gum log --level warn "Deleted: $name"; }
            ;;
        r)
            local new
            new=$(gum input --value="$name" --placeholder="new name…" --prompt="  ") || return 0
            [[ -n "$new" && "$new" != "$name" ]] && mv "$file" "$dir/$new"
            ;;
        z)
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
