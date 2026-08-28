package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
)

// copyChoice is one entry in the `c` ("copy…") menu: the key that picks it, a
// label, and the exact text written to the system clipboard when it's chosen.
type copyChoice struct {
	key   string // press this to copy straight away, so `c d` copies the directory
	label string
	value string
}

// copyChoicesFor builds the copy-menu entries for the absolute paths abs. With
// several paths each choice is the newline-joined list, so it pastes as one
// line per entry. Relative paths are taken relative to the process's working
// directory (where `fe` was launched), falling back to the absolute path if
// that can't be computed.
func copyChoicesFor(abs []string) []copyChoice {
	rels := make([]string, len(abs))
	names := make([]string, len(abs))
	var dirs []string
	cwd, cwdErr := os.Getwd()
	for i, p := range abs {
		rels[i] = p
		if cwdErr == nil {
			if r, rerr := filepath.Rel(cwd, p); rerr == nil {
				rels[i] = r
			}
		}
		names[i] = filepath.Base(p)
		if d := filepath.Dir(p); !slices.Contains(dirs, d) {
			dirs = append(dirs, d) // one entry per distinct directory
		}
	}
	return []copyChoice{
		{"a", "absolute path", strings.Join(abs, "\n")},
		{"r", "relative path", strings.Join(rels, "\n")},
		{"n", "file name", strings.Join(names, "\n")},
		{"d", "directory", strings.Join(dirs, "\n")},
	}
}

// oneLine flattens a multi-path value for display in the single-line picker
// rows and status messages.
func oneLine(s string) string { return strings.ReplaceAll(s, "\n", " · ") }

// openCopyMenu builds the `c` menu for targets: pick what to put on the system
// clipboard (absolute/relative path, file name, or directory). It's a keyed
// menu rather than a filtered one — four fixed entries are quicker to answer
// with the key printed beside them than to narrow by typing.
func (m model) openCopyMenu(targets []string) (tea.Model, tea.Cmd) {
	choices := copyChoicesFor(targets)
	items := make([]string, len(choices))
	for i, ch := range choices {
		items[i] = fmt.Sprintf("%s  %-14s %s", ch.key, ch.label, oneLine(ch.value))
	}

	m.copyItems = choices
	m.pickerKind = pickCopy
	m.pickerTitle = "copy to clipboard  ·  " + describePaths(targets)
	m.pickerAll = items
	m.pickerCursor = 0
	m.pickerTop = 0
	m.mode = modePicker
	m.ti.SetValue("")
	m.pickerApplyFilter()
	return m, nil
}

// copyKeyFor finds the choice a shortcut key picks.
func (m model) copyKeyFor(key string) (int, bool) {
	for i, ch := range m.copyItems {
		if ch.key == key {
			return i, true
		}
	}
	return 0, false
}

// applyCopy writes the chosen entry's value to the system clipboard.
func (m *model) applyCopy(idx int) {
	if idx < 0 || idx >= len(m.copyItems) {
		return
	}
	ch := m.copyItems[idx]
	if err := clipboard.WriteAll(ch.value); err != nil {
		m.setStatus(lvlErr, "clipboard: %v", err)
		return
	}
	m.setStatus(lvlInfo, "Copied %s: %s", ch.label, oneLine(ch.value))
}
