# fe (TUI)

A rewrite of `fe` as a proper terminal UI in Go, using
[Bubble Tea](https://github.com/charmbracelet/bubbletea) and
[Lip Gloss](https://github.com/charmbracelet/lipgloss) (the same
[Charm](https://charm.sh) ecosystem as `gum`), themed Tokyo Night.

Unlike the original `fe.sh` — which rented its key handling from `fzf` and so
couldn't do multi-key bindings — this version owns its input loop, so real vim
motions like `gg` / `G` work. It ports every feature of the shell version.

> **Note:** this is a normal binary, so it does **not** change your shell's
> directory on quit (the shell-function `cd`-on-exit trick from `fe.sh` is gone
> by design). It's for browsing and file operations.

## Build

Needs Go ≥ 1.24.2.

```bash
cd fe-tui
go build -o fe .
# then put it on your PATH, e.g.
install -m755 fe ~/.local/bin/fe
```

Optional external tools (same as the shell version): `nvim` (edit), `xdg-open`
(open files / open dir in file manager), `zip` / `unzip`.

## Usage

```bash
fe          # browse the current directory
fe ~/code   # browse a specific directory
```

Each row shows the name plus a size and last-modified date (`YYYY-MM-DD HH:MM`);
directories and symlinks show `-` for size. The columns hide automatically on
very narrow terminals.

## Keybindings

| Key                | Action                                   |
|--------------------|------------------------------------------|
| `h` / `←`          | parent directory                         |
| `j` / `↓`          | down                                     |
| `k` / `↑`          | up                                       |
| `l` / `→` / `enter`| enter directory / open file              |
| `gg`               | go to top                                |
| `G`                | go to bottom                             |
| `ctrl-d` / `ctrl-u`| half page down / up                      |
| `O`                | open with… (searchable app menu)         |
| `e`                | edit in `nvim`                           |
| `E`                | open current dir in system file manager  |
| `y`                | yank (copy)                              |
| `x`                | cut                                      |
| `p`                | paste here                               |
| `d`                | delete (with confirmation)               |
| `r`                | rename                                   |
| `z`                | zip / unzip                              |
| `s` / `/`          | filter (type; `esc` exits)               |
| `t`                | sort by name / modified                  |
| `.`                | show / hide dotfiles                     |
| `D`                | show / hide directories                  |
| `f`                | deep find (recursive)                    |
| `n`                | 10 newest files (recursive)              |
| `m`                | bookmark current directory               |
| `b`                | jump to a bookmark (`ctrl-d` deletes)    |
| `?`                | toggle help                              |
| `q` / `ctrl-c`     | quit                                     |

In the filter and picker screens: type to narrow, `↑`/`↓` (or `ctrl-j`/`ctrl-k`)
to move, `enter` to select, `esc` to cancel.

### Open with (`O`)

`O` opens a searchable menu of applications for the highlighted file, rather
than a bare command prompt. The list is a curated set of common apps (editors,
browsers, image / media / PDF viewers, office suites, file managers) filtered
down to those actually found on your `PATH`, so you only see apps you have. Type
to fuzzy-filter, `enter` to launch.

- **Terminal apps** (`nvim`, `less`, `ranger`, …) take over the screen and the
  listing reloads when they exit.
- **GUI apps** (VS Code, Firefox, GIMP, …) launch detached in the background.
- **`Default app (xdg-open)`** sits at the top; **`Custom command…`** sits at
  the bottom and drops into a free-form command prompt (the old `O` behaviour).

To add or reorder entries, edit the `curatedApps` list in `openwith.go`.

Bookmarks are shared with the shell version — same file at
`${XDG_DATA_HOME:-~/.local/share}/fe/bookmarks`.

## Notes on parity with `fe.sh`

- The clipboard (yank/cut) is **in-memory** (single session) rather than the
  cross-invocation temp file the shell version used; the pending item shows in
  the status line instead of as a `[paste …]` row.
- Deep find and "newest files" are implemented natively in Go (no `fd`/`find`
  dependency).
- `O` (open with) presents a searchable app menu instead of the shell version's
  single command prompt; the typed-command prompt remains as the last entry.
