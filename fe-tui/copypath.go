package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
)

// copyChoice is one entry in the `c` ("copy…") menu: a label and the exact text
// written to the system clipboard when it's chosen.
type copyChoice struct {
	label string
	value string
}

// copyChoicesFor builds the copy-menu entries for the absolute path abs. The
// relative path is taken relative to the process's working directory (where
// `fe` was launched), falling back to the absolute path if that can't be
// computed.
func copyChoicesFor(abs string) []copyChoice {
	rel := abs
	if cwd, err := os.Getwd(); err == nil {
		if r, rerr := filepath.Rel(cwd, abs); rerr == nil {
			rel = r
		}
	}
	return []copyChoice{
		{"absolute path", abs},
		{"relative path", rel},
		{"file name", filepath.Base(abs)},
		{"directory", filepath.Dir(abs)},
	}
}

// openCopyMenu builds the searchable `c` menu for target: pick what to put on
// the system clipboard (absolute/relative path, file name, or directory).
func (m model) openCopyMenu(target string) (tea.Model, tea.Cmd) {
	choices := copyChoicesFor(target)
	items := make([]string, len(choices))
	for i, ch := range choices {
		items[i] = fmt.Sprintf("%-14s %s", ch.label, ch.value)
	}

	m.copyItems = choices
	m.pickerKind = pickCopy
	m.pickerTitle = "copy to clipboard  ·  " + filepath.Base(target)
	m.pickerAll = items
	m.pickerCursor = 0
	m.pickerTop = 0
	m.mode = modePicker
	m.ti.Prompt = "/ "
	m.ti.SetValue("")
	m.ti.Placeholder = ""
	m.pickerApplyFilter()
	return m, m.ti.Focus()
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
	m.setStatus(lvlInfo, "Copied %s: %s", ch.label, ch.value)
}
