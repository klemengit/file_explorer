# fe

A minimal, modal terminal file explorer — a thin, fast layer over
[`fzf`](https://github.com/junegunn/fzf) and [`gum`](https://github.com/charmbracelet/gum)
with a Tokyo Night theme. You navigate with `hjkl`, run file actions with single
keys, and press `s` to filter. It lives as a shell function, so when you quit it
leaves you in the directory you were browsing.

## Requirements

**Required**

- [`fzf`](https://github.com/junegunn/fzf) ≥ 0.36 (uses `--disabled`, `enable-search`, `unbind`/`rebind`)
- [`gum`](https://github.com/charmbracelet/gum) (prompts, confirms, logging)

**Optional**

- `nvim` — the `e` (edit) action
- `fd` — faster recursive `f` (find); falls back to `find`
- `xdg-open` — the default-app `open` action
- `zip` / `unzip` — the `z` action

## Install

```bash
git clone <repo-url> file_exp
cd file_exp
./install.sh
```

`install.sh` checks dependencies, then adds a `source` line for `fe.sh` to your
shell rc (`~/.bashrc` or `~/.zshrc`). It is idempotent — safe to re-run.

Then reload your shell:

```bash
source ~/.bashrc   # or ~/.zshrc
```

### Manual install

Add this to your shell rc, pointing at wherever you cloned the repo:

```bash
source /path/to/file_exp/fe.sh
```

## Usage

```bash
fe          # open in the current directory
fe ~/code   # open in a specific directory
```

`fe` is **modal**. It starts in *command mode*, where letters are commands (not
search). Press `s` to drop into *search mode* and type to filter; `esc` returns
to command mode.

## Keybindings

### Command mode (default)

| Key       | Action                                   |
|-----------|------------------------------------------|
| `h`       | go to parent directory (←)               |
| `j`       | move down (↓)                            |
| `k`       | move up (↑)                              |
| `l`       | enter directory / open file (→)          |
| `enter`   | open in default app, or enter directory  |
| `O`       | open with… (prompts for a command)       |
| `e`       | edit in `nvim`                           |
| `y`       | yank (copy) to clipboard                 |
| `x`       | cut to clipboard                         |
| `p`       | paste clipboard here                     |
| `d`       | delete (with confirmation)               |
| `r`       | rename                                   |
| `z`       | zip a file/dir, or unzip a `.zip`        |
| `s`       | filter the current directory             |
| `f`       | deep recursive find (from current dir)   |
| `q` / `ctrl-c` | quit                                |

`enter`/`l` open files in the **default application** (`xdg-open`). Use `e` to
open in `nvim`.

### Search mode (after pressing `s`)

| Key                     | Action                                  |
|-------------------------|-----------------------------------------|
| *(type)*                | filter the current directory            |
| `↑` `↓` / `ctrl-k` `ctrl-j` | move through matches                |
| `enter`                 | open / enter the highlighted match      |
| `esc`                   | clear filter, return to command mode    |

### Special rows

- `..` — the first row; selecting it (or pressing `h`) goes to the parent.
- `[paste …]` — appears when the clipboard holds something. Selecting it offers
  **paste here** or **dismiss** (clears the clipboard). You can also paste
  anywhere with `p`.

## How it works

`fe` runs `fzf` with `--disabled`, so typing does not filter by default — that
frees plain letters to be command keys. Each command key writes a verb to a temp
file and accepts, so the loop knows both the key pressed and the highlighted
item. The `s` key flips fzf into search mode (`enable-search` + `unbind` the
letters so they type); `esc` flips back (`disable-search` + `rebind`).

## Theme

Colors are inline Tokyo Night escapes near the top of `fe.sh` (`_FE_FZF_OPTS`
and the `_FE_*` palette variables). Edit those to retheme.
