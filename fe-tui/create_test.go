package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// typeInto feeds a string into the shared text input one rune at a time, the
// way a real prompt receives it.
func typeInto(t *testing.T, m model, s string) model {
	t.Helper()
	for _, r := range s {
		tm, _ := m.updatePrompt(keyRune(r))
		m = tm.(model)
	}
	return m
}

func enter(t *testing.T, m model) model {
	t.Helper()
	tm, _ := m.updatePrompt(tea.KeyMsg{Type: tea.KeyEnter})
	return tm.(model)
}

func TestCreateFileAndFolder(t *testing.T) {
	m, dir := selModel(t)

	// "a" opens the prompt rather than acting immediately.
	m = press(t, m, keyRune('a'))
	if m.mode != modeCreate {
		t.Fatalf("a should open the create prompt, mode = %v", m.mode)
	}
	m = enter(t, typeInto(t, m, "report.md"))
	if m.mode != modeBrowse {
		t.Error("enter should return to browse")
	}
	fi, err := os.Stat(filepath.Join(dir, "report.md"))
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	if fi.IsDir() {
		t.Error("a plain name should make a file, not a directory")
	}
	if fi.Size() != 0 {
		t.Errorf("new file should be empty, got %d bytes", fi.Size())
	}

	// A trailing slash asks for a directory instead.
	m = press(t, m, keyRune('a'))
	m = enter(t, typeInto(t, m, "drafts/"))
	fi, err = os.Stat(filepath.Join(dir, "drafts"))
	if err != nil {
		t.Fatalf("folder not created: %v", err)
	}
	if !fi.IsDir() {
		t.Error("a trailing / should make a directory")
	}
}

func TestCreateNestedMakesParents(t *testing.T) {
	m, dir := selModel(t)
	m = press(t, m, keyRune('a'))
	m = enter(t, typeInto(t, m, "drafts/v2/notes.md"))

	if _, err := os.Stat(filepath.Join(dir, "drafts", "v2", "notes.md")); err != nil {
		t.Fatalf("nested file not created: %v", err)
	}
	// The cursor should land on the top-level component, which is the only
	// part of the new path visible in this directory.
	if got := m.cur().rows[m.cur().cursor].name; got != "drafts" {
		t.Errorf("cursor on %q, want the new top-level folder 'drafts'", got)
	}
}

func TestCreateRefusesEscapingNames(t *testing.T) {
	m, dir := selModel(t)
	outside := filepath.Join(dir, "..", "escaped.txt")

	for _, name := range []string{"../escaped.txt", "/tmp/fe-absolute-test.txt", ".."} {
		m = press(t, m, keyRune('a'))
		m = enter(t, typeInto(t, m, name))
		if m.statusLv != lvlErr {
			t.Errorf("name %q should have been refused", name)
		}
	}
	if _, err := os.Lstat(outside); err == nil {
		t.Error("a '..' name escaped the current directory")
	}
	if _, err := os.Lstat("/tmp/fe-absolute-test.txt"); err == nil {
		os.Remove("/tmp/fe-absolute-test.txt")
		t.Error("an absolute name escaped the current directory")
	}
}

func TestCreateRefusesExistingName(t *testing.T) {
	m, dir := selModel(t)
	before, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}

	m = press(t, m, keyRune('a'))
	m = enter(t, typeInto(t, m, "a.txt"))

	if m.statusLv != lvlErr || !strings.Contains(m.status, "exists") {
		t.Errorf("clashing name should be refused, status = %q", m.status)
	}
	after, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Error("an existing file was truncated by the create prompt")
	}
}

func TestCreateEmptyNameDoesNothing(t *testing.T) {
	m, dir := selModel(t)
	m = press(t, m, keyRune('a'))
	m = enter(t, m) // enter on an empty prompt

	if m.mode != modeBrowse {
		t.Error("enter should still close the prompt")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Errorf("an empty name created something: %d entries, want 3", len(entries))
	}
}

func TestCreateEscCancels(t *testing.T) {
	m, dir := selModel(t)
	m = press(t, m, keyRune('a'))
	m = typeInto(t, m, "unwanted.txt")
	tm, _ := m.updatePrompt(tea.KeyMsg{Type: tea.KeyEsc})
	m = tm.(model)

	if m.mode != modeBrowse {
		t.Error("esc should return to browse")
	}
	if _, err := os.Lstat(filepath.Join(dir, "unwanted.txt")); err == nil {
		t.Error("esc created the file anyway")
	}
}
