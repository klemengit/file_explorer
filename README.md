# fe

A minimal, modal terminal file explorer with a Tokyo Night theme. You navigate
with `hjkl`, run file actions with single keys, and press `s` to filter.

There are **two implementations** in this repo. They share the same idea, the
same theme and the same bookmarks file, but they are separate programs with
different keybindings.

| | [`fe-tui/`](fe-tui) — Go | `fe.sh` — shell |
|---|---|---|
| Status | **current, recommended** | the original, still works |
| Built on | [Bubble Tea](https://github.com/charmbracelet/bubbletea) | [`fzf`](https://github.com/junegunn/fzf) + [`gum`](https://github.com/charmbracelet/gum) |
| Install | build one binary (needs Go) | source a shell function |
| Two panes side by side | yes | no |
| Multi-key bindings (`gg`, `G`) | yes | no — `fzf` owns the keys |
| Multi-selection (`V`, `space`) | yes | no |
| Create files / folders (`a`) | yes | no |
| External drives window (`M`) | yes — mount / unmount / eject, and it names whatever is keeping a busy drive from unmounting | no |
| Leaves your shell in the browsed dir | no | yes |

**Start with [`fe-tui/README.md`](fe-tui/README.md)** — that's the current
version, and its keybindings are *not* the ones listed further down this page.

```bash
cd fe-tui
go build -o fe .                 # needs Go ≥ 1.24.2
install -m755 fe ~/.local/bin/fe
```

The Go version is meant to replace the shell one: install it as `fe` and drop
the `source .../fe.sh` line from your shell rc. The one thing you give up is the
`cd`-on-quit trick — a normal binary can't change its parent shell's directory.

Bookmarks are shared between the two, one path per line in
`${XDG_DATA_HOME:-~/.local/share}/fe/bookmarks`, so you can switch without
losing them.

---

## The shell version (`fe.sh`)

Everything below documents `fe.sh` only — a thin, fast layer over `fzf` and
`gum` that lives as a shell function, so when you quit it leaves you in the
directory you were browsing.

### Requirements

**Required**

- [`fzf`](https://github.com/junegunn/fzf) ≥ 0.36 (uses `--disabled`, `enable-search`, `unbind`/`rebind`)
- [`gum`](https://github.com/charmbracelet/gum) (prompts, confirms, logging)

**Optional**

- `nvim` — the `e` (edit) action
- `fd` — faster recursive `f` (find); falls back to `find`
- `xdg-open` — the default-app `open` action and `g` (open dir in file manager)
- `zip` / `unzip` — the `z` action

### Install

#### One-liner

Install `fzf` and `gum` first (see [Requirements](#requirements)), then:

```bash
wget -qO- https://raw.githubusercontent.com/klemengit/file_explorer/main/install.sh | bash
```

or with curl:

```bash
curl -fsSL https://raw.githubusercontent.com/klemengit/file_explorer/main/install.sh | bash
```

This downloads `fe.sh` into `~/.local/share/fe/` and adds a `source` line to your
shell rc (`~/.bashrc` or `~/.zshrc`).

#### From a clone

```bash
git clone https://github.com/klemengit/file_explorer.git
cd file_explorer
./install.sh
```

`install.sh` checks dependencies, then adds a `source` line for the local
`fe.sh` to your shell rc. It is idempotent — safe to re-run.

Either way, reload your shell afterwards:

```bash
source ~/.bashrc   # or ~/.zshrc
```

#### Manual install

Add this to your shell rc, pointing at wherever `fe.sh` lives:

```bash
source /path/to/fe.sh
```

### Usage

```bash
fe          # open in the current directory
fe ~/code   # open in a specific directory
```

`fe` is **modal**. It starts in *command mode*, where letters are commands (not
search). Press `s` or `/` to drop into *search mode* and type to filter; `esc` returns
to command mode.

### Keybindings

#### Command mode (default)

| Key       | Action                                   |
|-----------|------------------------------------------|
| `h`       | go to parent directory (←)               |
| `j`       | move down (↓)                            |
| `k`       | move up (↑)                              |
| `l`       | enter directory / open file (→)          |
| `enter`   | open in default app, or enter directory  |
| `O`       | open with… (prompts for a command)       |
| `e`       | edit in `nvim`                           |
| `g`       | open current dir in system file manager  |
| `y`       | yank (copy) to clipboard                 |
| `x`       | cut to clipboard                         |
| `p`       | paste clipboard here                     |
| `d`       | delete (with confirmation)               |
| `r`       | rename                                   |
| `z`       | zip a file/dir, or unzip a `.zip`        |
| `s` / `/` | filter the current directory             |
| `t`       | toggle sort: name ↔ last modified        |
| `.`       | show / hide dotfiles                     |
| `D`       | show / hide directories                  |
| `f`       | deep recursive find (from current dir)   |
| `n`       | 10 newest files (recursive, newest first)|
| `m`       | bookmark the current directory           |
| `b`       | jump to a bookmark (`ctrl-d` deletes)    |
| `?`       | toggle the help panel                    |
| `q` / `ctrl-c` | quit                                |

`enter`/`l` open files in the **default application** (`xdg-open`). Use `e` to
open in `nvim`.

The `t`, `.` and `D` toggles are sticky for the session and combine freely (e.g.
hide dotfiles *and* sort by modified time). Dotfiles are hidden by default.

> Several of these differ in the Go version — there `g` is half of `gg`, `E`
> opens the file manager, and `n` cycles the current directory's sort order.
> See [`fe-tui/README.md`](fe-tui/README.md) for its table.

#### Search mode (after pressing `s` or `/`)

| Key                     | Action                                  |
|-------------------------|-----------------------------------------|
| *(type)*                | filter the current directory            |
| `↑` `↓` / `ctrl-k` `ctrl-j` | move through matches                |
| `enter`                 | open / enter the highlighted match      |
| `esc`                   | clear filter, return to command mode    |

#### Special rows

- `..` — the first row; selecting it (or pressing `h`) goes to the parent.
- `[paste …]` — appears when the clipboard holds something. Selecting it offers
  **paste here** or **dismiss** (clears the clipboard). You can also paste
  anywhere with `p`.

#### Bookmarks

Press `m` to bookmark the current directory and `b` to open a picker of saved
bookmarks (`enter` jumps to one, `ctrl-d` deletes the highlighted one). Stale
bookmarks (deleted directories) are reported when you try to jump. Bookmarks are
stored one path per line in `${XDG_DATA_HOME:-~/.local/share}/fe/bookmarks`.

### How it works

`fe.sh` runs `fzf` with `--disabled`, so typing does not filter by default —
that frees plain letters to be command keys. Each command key writes a verb to a
temp file and accepts, so the loop knows both the key pressed and the
highlighted item. The `s` key flips fzf into search mode (`enable-search` +
`unbind` the letters so they type); `esc` flips back (`disable-search` +
`rebind`). This is also why it can't have `gg`-style chords, and why the Go
version exists.

### Theme

Colors are inline Tokyo Night escapes near the top of `fe.sh` (`_FE_FZF_OPTS`
and the `_FE_*` palette variables). Edit those to retheme.
