package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// chooserModel is selModel run as a file dialog, answering into a temp file.
func chooserModel(t *testing.T, c chooser) (model, string) {
	t.Helper()
	m, dir := selModel(t)
	c.out = filepath.Join(t.TempDir(), "out")
	m.choose = &c
	m.layout()
	return m, dir
}

// answer reads what the dialog was told, "" when nothing was written.
func answer(t *testing.T, m model) string {
	t.Helper()
	b, err := os.ReadFile(m.choose.out)
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

var (
	keyEnter = tea.KeyMsg{Type: tea.KeyEnter}
	keyCtrlS = tea.KeyMsg{Type: tea.KeyCtrlS}
)

func TestChooserIsSinglePane(t *testing.T) {
	m, _ := chooserModel(t, chooser{})
	if m.panes[0].width != m.width-2 {
		t.Errorf("chooser pane width = %d, want the full %d", m.panes[0].width, m.width-2)
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.active != 0 {
		t.Error("tab should do nothing with one pane")
	}
}

func TestChooserEnterChoosesFile(t *testing.T) {
	m, dir := chooserModel(t, chooser{})
	m = press(t, m, keyRune('j')) // a.txt
	tm, cmd := m.updateBrowse(keyEnter)
	if cmd == nil {
		t.Fatal("choosing a file should quit")
	}
	if got, want := answer(t, tm.(model)), filepath.Join(dir, "a.txt")+"\n"; got != want {
		t.Errorf("answer = %q, want %q", got, want)
	}
}

func TestChooserEnterWalksIntoDirs(t *testing.T) {
	m, dir := chooserModel(t, chooser{})
	sub := filepath.Join(dir, "0sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	m.reload()
	m = press(t, m, keyRune('j'), keyEnter)
	if m.cur().dir != sub {
		t.Errorf("dir = %s, want %s", m.cur().dir, sub)
	}
	if answer(t, m) != "" {
		t.Error("walking into a folder should not answer the dialog")
	}
}

func TestChooserMultipleSelection(t *testing.T) {
	m, dir := chooserModel(t, chooser{multiple: true})
	m = press(t, m, keyRune('j'), tea.KeyMsg{Type: tea.KeySpace}, tea.KeyMsg{Type: tea.KeySpace})
	tm, _ := m.updateBrowse(keyEnter)
	want := filepath.Join(dir, "a.txt") + "\n" + filepath.Join(dir, "b.txt") + "\n"
	if got := answer(t, tm.(model)); got != want {
		t.Errorf("answer = %q, want %q", got, want)
	}
}

func TestChooserSingleRefusesSeveral(t *testing.T) {
	m, _ := chooserModel(t, chooser{})
	m = press(t, m, keyRune('j'), tea.KeyMsg{Type: tea.KeySpace}, tea.KeyMsg{Type: tea.KeySpace}, keyEnter)
	if answer(t, m) != "" {
		t.Error("a single-file dialog should not take two")
	}
	if !strings.Contains(m.status, "only one") {
		t.Errorf("status = %q", m.status)
	}
}

func TestChooserDirectory(t *testing.T) {
	m, dir := chooserModel(t, chooser{dirs: true})
	m = press(t, m, keyRune('j'), keyEnter) // a file: ignored
	if answer(t, m) != "" {
		t.Fatal("a file should not answer a folder dialog")
	}
	tm, _ := m.updateBrowse(keyCtrlS)
	if got := answer(t, tm.(model)); got != dir+"\n" {
		t.Errorf("answer = %q, want %q", got, dir+"\n")
	}
}

func TestChooserSaveAs(t *testing.T) {
	m, dir := chooserModel(t, chooser{saveName: "report.pdf"})
	m = press(t, m, keyCtrlS)
	if m.mode != modeSaveAs || m.ti.Value() != "report.pdf" {
		t.Fatalf("ctrl+s should offer the suggested name, mode %v value %q", m.mode, m.ti.Value())
	}
	m = enter(t, m)
	if got := answer(t, m); got != filepath.Join(dir, "report.pdf")+"\n" {
		t.Errorf("answer = %q", got)
	}
}

func TestChooserSaveOverExistingAsks(t *testing.T) {
	m, dir := chooserModel(t, chooser{saveName: "a.txt"})
	m = press(t, m, keyCtrlS)
	m = enter(t, m)
	if m.mode != modeConfirm {
		t.Fatalf("saving over a.txt should ask, mode = %v", m.mode)
	}
	tm, _ := m.updateConfirm(keyRune('n'))
	m = tm.(model)
	if answer(t, m) != "" {
		t.Fatal("declining should not answer")
	}
	// Enter on an existing file offers to replace it too.
	m = press(t, m, keyRune('j'), keyEnter)
	if m.mode != modeConfirm {
		t.Fatalf("enter on a file should ask to replace it, mode = %v", m.mode)
	}
	tm, _ = m.updateConfirm(keyRune('y'))
	if got := answer(t, tm.(model)); got != filepath.Join(dir, "a.txt")+"\n" {
		t.Errorf("answer = %q", got)
	}
}

func TestChooserQuitCancels(t *testing.T) {
	m, _ := chooserModel(t, chooser{})
	_, cmd := m.updateBrowse(keyRune('q'))
	if cmd == nil {
		t.Fatal("q should quit")
	}
	if answer(t, m) != "" {
		t.Error("quitting should leave the answer empty")
	}
}
