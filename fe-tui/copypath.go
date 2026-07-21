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

// copyChoice is one entry in the `c` ("copy…") menu: a label and the exact text
// written to the system clipboard when it's chosen.
type copyChoice struct {
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
		{"absolute path", strings.Join(abs, "\n")},
		{"relative path", strings.Join(rels, "\n")},
		{"file name", strings.Join(names, "\n")},
		{"directory", strings.Join(dirs, "\n")},
	}
}

// oneLine flattens a multi-path value for display in the single-line picker
// rows and status messages.
func oneLine(s string) string { return strings.ReplaceAll(s, "\n", " · ") }

// openCopyMenu builds the searchable `c` menu for targets: pick what to put on
// the system clipboard (absolute/relative path, file name, or directory).
func (m model) openCopyMenu(targets []string) (tea.Model, tea.Cmd) {
	choices := copyChoicesFor(targets)
	items := make([]string, len(choices))
	for i, ch := range choices {
		items[i] = fmt.Sprintf("%-14s %s", ch.label, oneLine(ch.value))
	}

	m.copyItems = choices
	m.pickerKind = pickCopy
	m.pickerTitle = "copy to clipboard  ·  " + describePaths(targets)
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
	m.setStatus(lvlInfo, "Copied %s: %s", ch.label, oneLine(ch.value))
}
