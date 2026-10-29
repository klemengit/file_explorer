package main

import (
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// fe can stand in for a desktop file dialog: `fe --choose OUT` browses as
// usual, but picking a file writes its path to OUT and quits instead of
// opening it. xdg-desktop-portal-termfilechooser runs it that way for the open
// and save dialogs browsers and other apps raise, and reads OUT back; an OUT
// left empty means the dialog was cancelled.
//
// A chooser shows one pane. The second pane is there to copy and move between,
// which a dialog never does, and the dialog's window is usually small.

// chooser is how fe was asked to choose. nil in the model means a normal run.
type chooser struct {
	out      string // where the chosen paths go, one per line
	multiple bool   // more than one entry may be chosen
	dirs     bool   // choosing directories rather than files
	saveName string // a save dialog, suggesting this file name
}

func (m model) choosing() bool { return m.choose != nil }

// notChoosing gates the commands that only make sense with two panes.
func notChoosing(m model) bool { return !m.choosing() }

// chooserHint is the footer's key line while choosing: the keys that answer
// the dialog first, then the few that get you about.
func (m model) chooserHint() string {
	c := m.choose
	switch {
	case c.saveName != "":
		return "SAVE '" + c.saveName + "' · ctrl+s save here · enter on a file replaces it · a new folder · / filter · q cancel"
	case c.dirs:
		return "CHOOSE FOLDER · enter go in · ctrl+s choose this folder · space select · q cancel"
	case c.multiple:
		return "OPEN FILES · enter choose · space select · ctrl+s choose selection · / filter · f find · q cancel"
	}
	return "OPEN FILE · enter choose · / filter · f find · b bookmarks · q cancel"
}

// chooseSelected is enter while choosing: directories are still walked into,
// but a file answers the dialog rather than being opened.
func (m model) chooseSelected() (tea.Model, tea.Cmd) {
	p := m.cur()
	r, ok := p.current()
	if !ok {
		return m, nil
	}
	if !r.isParent && len(p.marked) == 0 && !p.visual {
		target := filepath.Join(p.dir, r.name)
		if info, err := os.Stat(target); err == nil && !info.IsDir() {
			switch {
			case m.choose.saveName != "":
				m.confirmSave(target)
				return m, nil
			case m.choose.dirs:
				return m, nil // files can't answer a folder dialog
			}
			return m.finishChoice([]string{target})
		}
	}
	// A selection answers an open dialog; enter on anything else navigates.
	if (len(p.marked) > 0 || p.visual) && m.choose.saveName == "" {
		return m.chooseHere()
	}
	return m.openSelected()
}

// chooseHere is ctrl+s: save into this directory, choose it as the folder, or
// choose the selection.
func (m model) chooseHere() (tea.Model, tea.Cmd) {
	p := m.cur()
	if m.choose.saveName != "" {
		m.startPrompt(modeSaveAs, "in "+abbrevHome(p.dir), "file name", m.choose.saveName)
		return m, nil
	}
	targets := p.targets()
	if m.choose.dirs && len(p.marked) == 0 && !p.visual {
		targets = []string{p.dir}
	}
	var picked []string
	for _, t := range targets {
		info, err := os.Stat(t)
		if err == nil && info.IsDir() == m.choose.dirs {
			picked = append(picked, t)
		}
	}
	switch {
	case len(picked) == 0:
		what := "file"
		if m.choose.dirs {
			what = "folder"
		}
		m.setStatus(lvlWarn, "Nothing to choose — put the cursor on a %s", what)
		return m, nil
	case len(picked) > 1 && !m.choose.multiple:
		m.setStatus(lvlWarn, "This dialog takes only one entry")
		return m, nil
	}
	return m.finishChoice(picked)
}

// applySaveAs answers the save-as prompt, asking first when the name is taken.
func (m model) applySaveAs(name string) (tea.Model, tea.Cmd) {
	if name == "" {
		return m, nil
	}
	if strings.ContainsRune(name, filepath.Separator) {
		m.setStatus(lvlErr, "save: a name, not a path — go to the folder first")
		return m, nil
	}
	target := filepath.Join(m.cur().dir, name)
	if info, err := os.Stat(target); err == nil {
		if info.IsDir() {
			m.setStatus(lvlErr, "save: %s is a folder", name)
			return m, nil
		}
		m.confirmSave(target)
		return m, nil
	}
	return m.finishChoice([]string{target})
}

func (m *model) confirmSave(target string) {
	m.confirmKind = confirmSave
	m.confirmPaths = []string{target}
	m.confirmMsg = "Replace '" + filepath.Base(target) + "'?"
	m.mode = modeConfirm
}

// finishChoice hands the paths to whoever asked and quits.
func (m model) finishChoice(paths []string) (tea.Model, tea.Cmd) {
	data := strings.Join(paths, "\n") + "\n"
	if err := os.WriteFile(m.choose.out, []byte(data), 0o600); err != nil {
		m.setStatus(lvlErr, "choose: %v", err)
		return m, nil
	}
	return m, tea.Quit
}
