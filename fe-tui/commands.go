package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Every browse command lives in commandSet, once. The key dispatch runs it, the
// help window lists it and the command palette searches it, all from the same
// entry — which is the point: the help list used to be a hand-written second
// copy of the key switch, and the two had already drifted apart (pgup and
// pgdown worked but were documented nowhere).

// command is one thing fe can do from the browse view.
type command struct {
	// keys that run it. The first is the canonical one; the rest are aliases,
	// and all of them are shown in the help.
	keys []string

	// keyLabel overrides how those keys read in the help and the palette. Only
	// quit needs it, for the ctrl-c that is handled before the browse view
	// ever sees it.
	keyLabel string

	// desc says what the command does. It is both the help text and what the
	// palette matches on and displays, so it reads as a verb phrase.
	desc string

	// alt holds extra words the palette matches but does not show — the names
	// you might reach for that aren't in desc ("mkdir" for new, "archive" for
	// zip).
	alt string

	// hidden keeps a command out of the palette while leaving it in the help.
	// The motions live here: a fuzzy list is a slow way to press j.
	hidden bool

	// when reports whether the command can do anything right now. A command
	// that can't is still listed, greyed out, so the palette answers "can fe do
	// this at all?" as well as "do it". nil means always.
	when func(model) bool

	// run does the work. It gets the model by value, exactly as the key switch
	// used to, and returns it the same way.
	run func(model) (tea.Model, tea.Cmd)
}

// hasTarget reports whether the active pane has something to act on — a
// selection, or a real entry under the cursor.
func hasTarget(m model) bool { return len(m.cur().targets()) > 0 }

// hasClip reports whether anything has been yanked or cut.
func hasClip(m model) bool { return m.clip != nil }

// commandSet is every browse command, in the order the help lists them:
// moving about, then selecting, then acting on files, then the views.
//
// It is filled in by init rather than by its own declaration because the
// command palette lists commandSet, so a literal naming openPalette would be a
// variable defined in terms of itself, which Go rejects.
var commandSet []command

func init() {
	commandSet = []command{
		{keys: []string{"tab"}, desc: "switch active pane", run: func(m model) (tea.Model, tea.Cmd) {
			m.active = 1 - m.active
			return m, nil
		}},
		{keys: []string{"f5"}, desc: "copy to other pane", when: hasTarget, run: func(m model) (tea.Model, tea.Cmd) {
			m.transfer(false)
			return m, nil
		}},
		{keys: []string{"f6"}, desc: "move to other pane", when: hasTarget, run: func(m model) (tea.Model, tea.Cmd) {
			m.transfer(true)
			return m, nil
		}},
		{keys: []string{"h", "left"}, desc: "parent directory", run: func(m model) (tea.Model, tea.Cmd) {
			m.goParent()
			return m, nil
		}},
		{keys: []string{"l", "right", "enter"}, desc: "enter directory / open file", run: func(m model) (tea.Model, tea.Cmd) {
			return m.openSelected()
		}},
		{keys: []string{"j", "down"}, desc: "down", hidden: true, run: func(m model) (tea.Model, tea.Cmd) {
			m.cur().move(1)
			return m, nil
		}},
		{keys: []string{"k", "up"}, desc: "up", hidden: true, run: func(m model) (tea.Model, tea.Cmd) {
			m.cur().move(-1)
			return m, nil
		}},
		{keys: []string{"G", "end"}, desc: "go to bottom", hidden: true, run: func(m model) (tea.Model, tea.Cmd) {
			p := m.cur()
			p.cursor = len(p.rows) - 1
			p.clampScroll()
			return m, nil
		}},
		{keys: []string{"ctrl+d"}, desc: "half page down", hidden: true, run: func(m model) (tea.Model, tea.Cmd) {
			m.cur().move(m.cur().listHeight() / 2)
			return m, nil
		}},
		{keys: []string{"ctrl+u"}, desc: "half page up", hidden: true, run: func(m model) (tea.Model, tea.Cmd) {
			m.cur().move(-m.cur().listHeight() / 2)
			return m, nil
		}},
		{keys: []string{"pgdown"}, desc: "page down", hidden: true, run: func(m model) (tea.Model, tea.Cmd) {
			m.cur().move(m.cur().listHeight())
			return m, nil
		}},
		{keys: []string{"pgup"}, desc: "page up", hidden: true, run: func(m model) (tea.Model, tea.Cmd) {
			m.cur().move(-m.cur().listHeight())
			return m, nil
		}},

		{keys: []string{"V"}, desc: "visual select (j/k extend, V keeps)", run: func(m model) (tea.Model, tea.Cmd) {
			// Vim-style linewise visual selection: V starts it at the cursor, j/k
			// extend it, and a second V commits the range to the selection.
			p := m.cur()
			if p.visual {
				p.commitVisual()
				m.setStatus(lvlInfo, "%d selected", len(p.selectedPaths()))
			} else {
				p.visual = true
				p.anchor = p.cursor
			}
			return m, nil
		}},
		{keys: []string{" ", "space"}, desc: "select / deselect, move down", hidden: true, run: func(m model) (tea.Model, tea.Cmd) {
			// Mark the row and step down, so tapping space runs down a block.
			p := m.cur()
			p.toggleMark()
			p.move(1)
			return m, nil
		}},
		{keys: []string{"esc"}, desc: "leave visual / clear selection", hidden: true, run: func(m model) (tea.Model, tea.Cmd) {
			// First esc drops a live visual range, a second clears the marks.
			p := m.cur()
			if p.visual {
				p.visual = false
			} else if len(p.marked) > 0 {
				p.clearSelection()
			}
			return m, nil
		}},

		{keys: []string{"O"}, desc: "open with… (app menu)", when: hasTarget, run: func(m model) (tea.Model, tea.Cmd) {
			return m.openOpenWith(m.cur().targets())
		}},
		{keys: []string{"e"}, desc: "edit in nvim", when: hasTarget, run: func(m model) (tea.Model, tea.Cmd) {
			c := exec.Command("nvim", m.cur().targets()...)
			return m, tea.ExecProcess(c, func(err error) tea.Msg { return editFinishedMsg{err} })
		}},
		{keys: []string{"E"}, desc: "open current dir in file manager", run: func(m model) (tea.Model, tea.Cmd) {
			if err := openDetached("xdg-open", m.cur().dir); err != nil {
				m.setStatus(lvlErr, "xdg-open: %v", err)
			}
			return m, nil
		}},
		{keys: []string{"y"}, desc: "yank (copy)", when: hasTarget, run: func(m model) (tea.Model, tea.Cmd) {
			p := m.cur()
			targets := p.targets()
			m.clip = &clipEntry{paths: targets, cut: false}
			m.setStatus(lvlInfo, "Yanked: %s", describePaths(targets))
			p.clearSelection()
			return m, nil
		}},
		{keys: []string{"x"}, desc: "cut", when: hasTarget, run: func(m model) (tea.Model, tea.Cmd) {
			p := m.cur()
			targets := p.targets()
			m.clip = &clipEntry{paths: targets, cut: true}
			m.setStatus(lvlWarn, "Cut: %s", describePaths(targets))
			p.clearSelection()
			return m, nil
		}},
		{keys: []string{"p"}, desc: "paste here", when: hasClip, run: func(m model) (tea.Model, tea.Cmd) {
			m.paste()
			return m, nil
		}},
		{keys: []string{"c"}, desc: "copy path / name to clipboard", when: hasTarget, run: func(m model) (tea.Model, tea.Cmd) {
			return m.openCopyMenu(m.cur().targets())
		}},
		{keys: []string{"d"}, desc: "delete (confirm)", alt: "remove trash", when: hasTarget, run: func(m model) (tea.Model, tea.Cmd) {
			targets := m.cur().targets()
			m.confirmKind = confirmDelete
			m.confirmPaths = targets
			m.confirmMsg = fmt.Sprintf("Delete %s?", describeQuoted(targets))
			m.mode = modeConfirm
			return m, nil
		}},
		{keys: []string{"r"}, desc: "rename", alt: "move mv", when: hasTarget, run: func(m model) (tea.Model, tea.Cmd) {
			target, ok := m.cur().selectedTarget()
			if !ok {
				return m, nil
			}
			m.startPrompt(modeRename, "renaming "+describeQuoted([]string{target}),
				"new name…", filepath.Base(target))
			return m, nil
		}},
		{keys: []string{"a"}, desc: "new file (or folder, end with /)", alt: "mkdir touch create", run: func(m model) (tea.Model, tea.Cmd) {
			m.startPrompt(modeCreate, "in "+abbrevHome(m.cur().dir),
				"name, end with / for a folder…", "")
			return m, nil
		}},
		{keys: []string{"z"}, desc: "zip / unzip", alt: "archive compress extract", when: hasTarget, run: func(m model) (tea.Model, tea.Cmd) {
			m.zip()
			return m, nil
		}},

		{keys: []string{"s", "/"}, desc: "filter (type; esc exits)", alt: "search", run: func(m model) (tea.Model, tea.Cmd) {
			m.mode = modeFilter
			m.ti.SetValue("")
			m.ti.Prompt = "/ "
			m.applyView()
			return m, m.ti.Focus()
		}},
		{keys: []string{"f"}, desc: "deep find", alt: "search recursive", run: func(m model) (tea.Model, tea.Cmd) {
			return m.openPicker(pickFind)
		}},
		{keys: []string{"t"}, desc: "sort name / newest", run: func(m model) (tea.Model, tea.Cmd) {
			// Toggle between name order and newest-first.
			p := m.cur()
			if p.sortMode == sortName {
				p.sortMode = sortTimeDesc
			} else {
				p.sortMode = sortName
			}
			m.reload()
			return m, nil
		}},
		{keys: []string{"n"}, desc: "cycle sort: newest → oldest → name", run: func(m model) (tea.Model, tea.Cmd) {
			p := m.cur()
			switch p.sortMode {
			case sortName:
				p.sortMode = sortTimeDesc
			case sortTimeDesc:
				p.sortMode = sortTimeAsc
			default:
				p.sortMode = sortName
			}
			p.cursor, p.top = 0, 0
			m.reload()
			return m, nil
		}},
		{keys: []string{"."}, desc: "show / hide dotfiles", alt: "hidden", run: func(m model) (tea.Model, tea.Cmd) {
			p := m.cur()
			p.showDots = !p.showDots
			m.reload()
			return m, nil
		}},
		{keys: []string{"D"}, desc: "show / hide directories", run: func(m model) (tea.Model, tea.Cmd) {
			p := m.cur()
			p.showDirs = !p.showDirs
			m.reload()
			return m, nil
		}},
		{keys: []string{"m"}, desc: "bookmark dir", run: func(m model) (tea.Model, tea.Cmd) {
			p := m.cur()
			added, err := addBookmark(p.dir)
			switch {
			case err != nil:
				m.setStatus(lvlErr, "bookmark: %v", err)
			case added:
				m.setStatus(lvlInfo, "Bookmarked: %s", p.dir)
			default:
				m.setStatus(lvlInfo, "Already bookmarked")
			}
			return m, nil
		}},
		{keys: []string{"b"}, desc: "jump to bookmark", run: func(m model) (tea.Model, tea.Cmd) {
			return m.openPicker(pickBookmarks)
		}},
		{keys: []string{"M"}, desc: "external drives (u unmount, e eject)", alt: "usb mount", run: func(m model) (tea.Model, tea.Cmd) {
			return m.openDrives()
		}},
		{keys: []string{":"}, desc: "command palette", alt: "commands menu search", hidden: true, run: func(m model) (tea.Model, tea.Cmd) {
			return m.openPalette()
		}},
		{keys: []string{"?"}, desc: "toggle this help", alt: "keys bindings", run: func(m model) (tea.Model, tea.Cmd) {
			m.showHelp = true
			m.helpTop = 0
			return m, nil
		}},
		{keys: []string{"q"}, keyLabel: "q / ctrl-c", desc: "quit", alt: "exit", run: func(m model) (tea.Model, tea.Cmd) {
			m.saveSession()
			return m, tea.Quit
		}},
	}
}

// commandFor looks up the command a key runs.
func commandFor(key string) (command, bool) {
	for _, c := range commandSet {
		for _, k := range c.keys {
			if k == key {
				return c, true
			}
		}
	}
	return command{}, false
}

// available reports whether the command can do anything in this model.
func (c command) available(m model) bool { return c.when == nil || c.when(m) }

// prettyKeys renders a command's keys the way the help and the palette show
// them: arrows for the cursor keys, a dash in the ctrl combinations, and the
// aliases joined with slashes.
func (c command) prettyKeys() string {
	if c.keyLabel != "" {
		return c.keyLabel
	}
	seen := make(map[string]bool, len(c.keys))
	out := make([]string, 0, len(c.keys))
	for _, k := range c.keys {
		p := prettyKey(k)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return strings.Join(out, " / ")
}

// prettyKey names a single key for display.
func prettyKey(k string) string {
	switch k {
	case "up":
		return "↑"
	case "down":
		return "↓"
	case "left":
		return "←"
	case "right":
		return "→"
	case " ", "space":
		return "space"
	}
	return strings.ReplaceAll(k, "ctrl+", "ctrl-")
}
