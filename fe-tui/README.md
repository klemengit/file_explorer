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

## Build & install

Needs Go ≥ 1.24.2.

```bash
cd fe-tui
go build -o fe .                 # produces the self-contained ./fe binary
install -m755 fe ~/.local/bin/fe # put it on your PATH
```

The result is a single static-ish binary (only links libc), so once installed it
runs on its own — the Go toolchain is only needed to build it.

This binary is meant to **replace** the shell version: install it as `fe` and
drop the `source .../fe.sh` line from your shell rc. (Because it's a normal
binary it can't `cd` your shell on quit — see the note at the top.)

After editing the source (for example adding apps to `curatedApps` in
`openwith.go`), just re-run the two commands above to rebuild and reinstall.

Optional external tools (same as the shell version): `nvim` (edit), `xdg-open`
(open files / open dir in file manager), `zip` / `unzip`, plus `lsblk` and
`udisksctl` for the `M` drives window. The `O` open-with menu picks up any of a
broad set of apps found on your `PATH`.

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
| `V`                | visual select (`j`/`k` extend, `V` keeps)|
| `space`            | select / deselect, then move down        |
| `esc`              | leave visual mode / clear the selection  |
| `O`                | open with… (searchable app menu)         |
| `e`                | edit in `nvim`                           |
| `E`                | open current dir in system file manager  |
| `y`                | yank (copy)                              |
| `x`                | cut                                      |
| `p`                | paste here                               |
| `c`                | copy path / name to clipboard (menu)     |
| `d`                | delete (with confirmation)               |
| `r`                | rename                                   |
| `z`                | zip / unzip                              |
| `s` / `/`          | filter (type; `esc` exits)               |
| `t`                | sort by name / newest                    |
| `.`                | show / hide dotfiles                     |
| `D`                | show / hide directories                  |
| `f`                | deep find (recursive)                    |
| `n`                | cycle sort: newest → oldest → name        |
| `m`                | bookmark current directory               |
| `b`                | jump to a bookmark (`ctrl-d` deletes)    |
| `M`                | external drives (mount / unmount / eject)|
| `?`                | toggle help                              |
| `q` / `ctrl-c`     | quit                                     |

In the filter and picker menus: type to narrow, `↑`/`↓` (or `ctrl-j`/`ctrl-k`)
to move, `enter` to select, `esc` to cancel.

### Floating windows

The menus are small windows that float in the middle of the screen with both
panes still visible around them, rather than pages that take the screen over:
`?` (help), `O` (open with), `c` (copy to clipboard), `b` (bookmarks) and `M`
(drives). Each one sizes itself to its contents and to your terminal.

The one exception is `f` (deep find), which stays full-screen: it lists every
file under the current directory, so it wants all the room it can get.

The help window lays the bindings out in two columns when the terminal is wide
enough and falls back to one column when it isn't; if the list still doesn't
fit, `j`/`k` (and `ctrl-d`/`ctrl-u`, `g`/`G`) scroll it.

### Multi-selection (`V`, `space`)

Two vim-flavoured ways to select several entries at once:

- **`V`** starts a linewise visual selection at the cursor. `j`/`k` (and every
  other motion) extend or shrink it live. A second `V` keeps the range as a
  selection; `esc` throws it away.
- **`space`** toggles the entry under the cursor and steps down one row, so you
  can tap it repeatedly to pick out a block.

Selected rows are green and carry a `*` in the gutter next to the cursor arrow;
the pane header shows `[2 selected]`, and `[VISUAL]` while a range is live.
Pressing `esc` with no live range clears the selection.

Every action that can sensibly work on more than one entry acts on the whole
selection: `y` / `x` / `p` (yank, cut, paste), `d` (delete — one confirmation
for the batch), `F5` / `F6` (copy / move to the other pane), `z` (zip),
`c` (copy paths to the clipboard, one per line), `e` (open all in `nvim`) and
`O` (open with…). With nothing selected they act on the row under the cursor
exactly as before, so nothing changes when you don't use the feature.

The action **consumes** the selection: once it has run, the marks are cleared
(a partially failed run keeps whatever it didn't get to). The selection is also
per-directory — it is dropped when the pane navigates elsewhere — and marks
hidden by an active filter are left out, so a bulk action never touches
something you can't see.

`..` can never be selected.

### Zipping several entries (`z`)

With one entry selected (or none), `z` behaves as before: it zips
`foo` → `foo.zip`, or unzips a `.zip`. With several entries selected it asks for
an archive name — prefilled with the current directory's name — and packs them
all into that one archive.

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

### External drives (`M`)

`M` opens a small window floating over the two panes that lists your external
drives — USB sticks, SD cards, external disks: anything `lsblk` reports as
removable or hotplug. Internal partitions and your root filesystem are
deliberately left out, so there is no way to eject the disk you are running on.

Each row shows the drive's label (falling back to its vendor and model), its
size, its filesystem and where it is mounted; the line underneath gives the
device node and how much space is free.

This window is modal: only its own keys do anything, so the browse commands
(`y`, `p`, `d`, `z`, …) stay inert while it is open.

| Key             | Action                                              |
|-----------------|-----------------------------------------------------|
| `j` / `k`       | move                                                |
| `enter` / `l`   | open the drive in the active pane, mounting it first if needed |
| `u`             | unmount                                             |
| `e`             | eject — unmount, then power the device off          |
| `r`             | re-read the drive list                              |
| `esc` / `q`     | close                                               |

Unmounted drives are listed too: `enter` mounts them (via `udisksctl`, which
needs no root) and jumps straight in. `u` just unmounts; `e` additionally powers
the device down so it is safe to unplug — and if the hardware refuses the
power-off, the status line says the drive was only unmounted rather than
claiming otherwise.

Mount and eject run in the background, so the window stays drawn and reports
`Ejecting …` while it works; keys are ignored until it finishes. If a pane was
browsing inside a drive you just unmounted, that pane falls back to your home
directory instead of showing a directory that no longer exists.

Requires `lsblk` (util-linux, present on any Linux) for the listing, and
`udisksctl` (udisks2) for mounting and powering off; `umount` and `eject` are
used as fallbacks where they can stand in.

### Copy to clipboard (`c`)

`c` opens a small menu of things to copy to the **system clipboard** for the
highlighted entry: its **absolute path**, its **relative path** (relative to the
directory `fe` was launched from), its **file name**, or its **directory**. Each
row previews the exact text; `enter` copies it.

With several entries selected each row copies the whole list — one path (or
name) per line — and the menu previews them joined with `·`.

This uses the system clipboard (via `wl-copy` / `xclip` / `xsel`), unlike the
in-memory `y`/`x`/`p` yank-and-paste — so you can paste the path into other
programs.

Bookmarks are shared with the shell version — same file at
`${XDG_DATA_HOME:-~/.local/share}/fe/bookmarks`.

## Notes on parity with `fe.sh`

- The clipboard (yank/cut) is **in-memory** (single session) rather than the
  cross-invocation temp file the shell version used; the pending item shows in
  the status line instead of as a `[paste …]` row.
- Deep find (`f`) is implemented natively in Go (no `fd`/`find` dependency).
- `n` cycles the current directory's ordering: **newest first → oldest first →
  original (name) order** (a flat, `ls -t`-style sort over the current
  directory), rather than the shell version's recursive newest-files list. The
  header shows `[newest]` or `[oldest]` while a time sort is active.
- `O` (open with) presents a searchable app menu instead of the shell version's
  single command prompt; the typed-command prompt remains as the last entry.
