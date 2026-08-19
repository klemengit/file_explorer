package main

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// cursorName is what the active pane is highlighting, for readable assertions.
func cursorName(m model) string {
	r, ok := m.cur().current()
	if !ok {
		return "<none>"
	}
	if r.isParent {
		return ".."
	}
	return r.name
}

// Esc is the way out that keeps the match: the full list comes back and the
// cursor stays on whatever the filter narrowed to, ready for r, d or y.
func TestFilterEscKeepsTheMatch(t *testing.T) {
	m, _ := selModel(t) // rows: .., a.txt, b.txt, c.txt

	m = pressAll(t, m, '/', 'c')
	if got := cursorName(m); got != "c.txt" {
		t.Fatalf("filter left the cursor on %q, want c.txt", got)
	}

	tm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = tm.(model)

	if m.mode != modeBrowse {
		t.Fatalf("esc did not leave filter mode (mode %d)", m.mode)
	}
	if len(m.cur().rows) != len(m.cur().allRows) {
		t.Fatalf("esc left the list narrowed: %d of %d rows", len(m.cur().rows), len(m.cur().allRows))
	}
	if got := cursorName(m); got != "c.txt" {
		t.Fatalf("esc landed on %q, want c.txt", got)
	}
}

// A filter that matches nothing has no match to keep, so esc puts the cursor
// back where it was when the filter opened rather than dropping it at the top.
func TestFilterEscFallsBackToOrigin(t *testing.T) {
	m, _ := selModel(t)
	m = press(t, m, keyRune('j'), keyRune('j')) // onto b.txt
	if got := cursorName(m); got != "b.txt" {
		t.Fatalf("setup left the cursor on %q, want b.txt", got)
	}

	m = pressAll(t, m, '/', 'z', 'z', 'z')
	if len(m.cur().rows) != 0 {
		t.Fatalf("zzz matched %d rows, want none", len(m.cur().rows))
	}

	tm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = tm.(model)

	if got := cursorName(m); got != "b.txt" {
		t.Fatalf("esc after a dead filter landed on %q, want b.txt", got)
	}
}

// Opening a file from the filter unnarrows the pane, so the cursor — an index
// into the list that just grew back — has to be put on the file by name.
func TestFilterEnterOnFileKeepsTheCursorOnIt(t *testing.T) {
	// Opening a file hands it to xdg-open. An empty PATH makes that fail
	// harmlessly instead of throwing a text editor onto whoever runs the
	// tests; the cursor is placed either way.
	t.Setenv("PATH", "")

	m, _ := selModel(t)

	m = pressAll(t, m, '/', 'c')
	tm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = tm.(model)

	if m.mode != modeBrowse {
		t.Fatalf("enter did not leave filter mode (mode %d)", m.mode)
	}
	if got := cursorName(m); got != "c.txt" {
		t.Fatalf("enter on a file left the cursor on %q, want c.txt", got)
	}
}

// Enter on a directory still enters it, which places its own cursor at the
// top — restoring the cursor by name must not fight that.
func TestFilterEnterOnDirEntersIt(t *testing.T) {
	m, dir := selModel(t)
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := m.cur().reload(""); err != nil {
		t.Fatal(err)
	}

	m = pressAll(t, m, '/', 's', 'u', 'b')
	tm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = tm.(model)

	if m.cur().dir != sub {
		t.Fatalf("enter on a directory left the pane in %q, want %q", m.cur().dir, sub)
	}
	if m.cur().cursor != 0 {
		t.Fatalf("entering a directory left the cursor at %d, want 0", m.cur().cursor)
	}
}

// s used to be a second name for /, which meant the filter had two keys and
// the letter could never be given to anything else.
func TestFilterHasOnlyOneKey(t *testing.T) {
	m, _ := selModel(t)

	if mm := pressAll(t, m, 's'); mm.mode == modeFilter {
		t.Error("s still opens the filter")
	}
	if mm := pressAll(t, m, '/'); mm.mode != modeFilter {
		t.Errorf("/ did not open the filter (mode %d)", mm.mode)
	}
}
