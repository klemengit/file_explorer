package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// selModel builds a model over a temp dir holding a.txt, b.txt and c.txt, so
// rows are [.., a.txt, b.txt, c.txt] in name order.
func selModel(t *testing.T) (model, string) {
	t.Helper()
	dir := t.TempDir()
	for _, n := range []string{"a.txt", "b.txt", "c.txt"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := newModel(dir)
	m.width, m.height = 80, 24
	m.layout()
	return m, dir
}

// press drives keys through the browse handler, threading the model along.
func press(t *testing.T, m model, keys ...tea.KeyMsg) model {
	t.Helper()
	for _, k := range keys {
		tm, _ := m.updateBrowse(k)
		m = tm.(model)
	}
	return m
}

// pressCopy drives keys like press but also runs the command each key returns,
// feeding the messages it produces back through Update. A background copy
// started by p or F5 actually runs to completion this way.
func pressCopy(t *testing.T, m model, keys ...tea.KeyMsg) model {
	t.Helper()
	for _, k := range keys {
		tm, cmd := m.updateBrowse(k)
		m = tm.(model)
		m = drainCmd(t, m, cmd)
	}
	return m
}

// drainCmd executes cmd the way the bubbletea program would: run it, hand the
// message it produces to Update, and recurse into whatever commands those
// produce — until nothing is left to run.
func drainCmd(t *testing.T, m model, cmd tea.Cmd) model {
	t.Helper()
	if cmd == nil {
		return m
	}
	return drainMsg(t, m, cmd())
}

// drainMsg hands msg to Update and then drains any command that came back
// with it. A BatchMsg ([]Cmd) is unwrapped and each command run in turn.
func drainMsg(t *testing.T, m model, msg tea.Msg) model {
	t.Helper()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			m = drainCmd(t, m, c)
		}
		return m
	}
	tm, next := m.Update(msg)
	m = tm.(model)
	return drainCmd(t, m, next)
}

func keyRune(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

// baseNames reduces a path list to base names for readable assertions.
func baseNames(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = filepath.Base(p)
	}
	return out
}

// TestVisualSelectExtends checks V starts a linewise selection at the cursor
// and j extends it, so targets covers the whole range while it is live.
func TestVisualSelectExtends(t *testing.T) {
	m, _ := selModel(t)
	m = press(t, m, keyRune('j')) // onto a.txt
	m = press(t, m, keyRune('V'), keyRune('j'))

	if !m.cur().visual {
		t.Fatal("V should enter visual mode")
	}
	if got, want := baseNames(m.cur().targets()), []string{"a.txt", "b.txt"}; !equalStrs(got, want) {
		t.Fatalf("visual targets = %v, want %v", got, want)
	}
	// Shrinking the range back past the anchor tracks the cursor too.
	m = press(t, m, keyRune('k'), keyRune('k'))
	if got, want := baseNames(m.cur().targets()), []string{"a.txt"}; !equalStrs(got, want) {
		t.Fatalf("after shrinking, targets = %v, want %v", got, want)
	}
}

// TestVisualCommitAndClear checks a second V keeps the range as marks, and that
// esc drops a live range first and clears the marks on the next press.
func TestVisualCommitAndClear(t *testing.T) {
	m, _ := selModel(t)
	m = press(t, m, keyRune('j'), keyRune('V'), keyRune('j'), keyRune('V'))

	if m.cur().visual {
		t.Fatal("second V should leave visual mode")
	}
	if got, want := baseNames(m.cur().selectedPaths()), []string{"a.txt", "b.txt"}; !equalStrs(got, want) {
		t.Fatalf("committed selection = %v, want %v", got, want)
	}

	// A fresh visual range on top of the marks: esc drops the range only.
	m = press(t, m, keyRune('V'), tea.KeyMsg{Type: tea.KeyEsc})
	if m.cur().visual {
		t.Fatal("esc should leave visual mode")
	}
	if n := len(m.cur().selectedPaths()); n != 2 {
		t.Fatalf("esc should keep committed marks, got %d selected", n)
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if n := len(m.cur().selectedPaths()); n != 0 {
		t.Fatalf("second esc should clear marks, got %d selected", n)
	}
}

// TestSpaceMarksAndMovesDown checks space toggles the row under the cursor and
// steps down, and that pressing it again on a marked row deselects. It sends
// both key spellings a terminal may produce for space.
func TestSpaceMarksAndMovesDown(t *testing.T) {
	m, _ := selModel(t)
	m = press(t, m, keyRune('j'))
	m = press(t, m, tea.KeyMsg{Type: tea.KeySpace})
	m = press(t, m, keyRune(' '))

	if got := m.cur().cursor; got != 3 {
		t.Fatalf("cursor after two spaces = %d, want 3 (c.txt)", got)
	}
	if got, want := baseNames(m.cur().selectedPaths()), []string{"a.txt", "b.txt"}; !equalStrs(got, want) {
		t.Fatalf("selection = %v, want %v", got, want)
	}

	// Back onto a.txt and press space again: it deselects.
	m.cur().cursorTo("a.txt")
	m = press(t, m, keyRune(' '))
	if got, want := baseNames(m.cur().selectedPaths()), []string{"b.txt"}; !equalStrs(got, want) {
		t.Fatalf("after deselect, selection = %v, want %v", got, want)
	}
}

// TestParentRowNeverSelected checks ".." can't be marked or swept into a
// visual range, so bulk actions can never target the parent directory.
func TestParentRowNeverSelected(t *testing.T) {
	m, _ := selModel(t)
	m = press(t, m, keyRune(' ')) // cursor starts on ".."
	if n := len(m.cur().selectedPaths()); n != 0 {
		t.Fatalf("space on '..' selected %d entries, want 0", n)
	}
	m.cur().cursor = 0
	m = press(t, m, keyRune('V'), keyRune('j'))
	if got, want := baseNames(m.cur().targets()), []string{"a.txt"}; !equalStrs(got, want) {
		t.Fatalf("visual range over '..' = %v, want %v", got, want)
	}
}

// TestTargetsFallBackToCursor checks that with nothing selected an action still
// acts on the row under the cursor — single-item behaviour is unchanged.
func TestTargetsFallBackToCursor(t *testing.T) {
	m, _ := selModel(t)
	m = press(t, m, keyRune('j'), keyRune('j'))
	if got, want := baseNames(m.cur().targets()), []string{"b.txt"}; !equalStrs(got, want) {
		t.Fatalf("unselected targets = %v, want %v", got, want)
	}
}

// TestSelectionClearedOnDirChange checks marks don't survive navigation — they
// are names, and would otherwise re-match same-named entries elsewhere.
func TestSelectionClearedOnDirChange(t *testing.T) {
	m, dir := selModel(t)
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	m.reload()

	m.cur().cursorTo("a.txt")
	m = press(t, m, keyRune(' '))
	if n := len(m.cur().selectedPaths()); n != 1 {
		t.Fatalf("setup: want 1 selected, got %d", n)
	}

	m.cur().cursorTo("sub")
	m = press(t, m, keyRune('l')) // into sub/
	if n := len(m.cur().selectedPaths()); n != 0 {
		t.Fatalf("entering a directory should clear the selection, got %d", n)
	}
	m = press(t, m, keyRune('h')) // back up
	if n := len(m.cur().selectedPaths()); n != 0 {
		t.Fatalf("going to the parent should clear the selection, got %d", n)
	}
}

// TestBulkDeleteSelection drives space-space-d-y and checks both marked files
// are gone, the unmarked one survives, and the selection is consumed.
func TestBulkDeleteSelection(t *testing.T) {
	m, dir := selModel(t)
	m = press(t, m, keyRune('j'), keyRune(' '), keyRune(' ')) // mark a.txt, b.txt
	m = press(t, m, keyRune('d'))

	if m.mode != modeConfirm {
		t.Fatalf("d should ask for confirmation, mode = %v", m.mode)
	}
	if m.confirmMsg != "Delete 2 entries?" {
		t.Fatalf("confirm message = %q", m.confirmMsg)
	}
	tm, _ := m.updateConfirm(keyRune('y'))
	m = tm.(model)

	for _, n := range []string{"a.txt", "b.txt"} {
		if _, err := os.Lstat(filepath.Join(dir, n)); !os.IsNotExist(err) {
			t.Errorf("%s should have been deleted", n)
		}
	}
	if _, err := os.Lstat(filepath.Join(dir, "c.txt")); err != nil {
		t.Errorf("c.txt was not selected and must survive: %v", err)
	}
	if n := len(m.cur().selectedPaths()); n != 0 {
		t.Fatalf("delete should consume the selection, got %d", n)
	}
}

// TestBulkYankPaste checks a multi-entry yank pastes every path into another
// directory and leaves the sources alone.
func TestBulkYankPaste(t *testing.T) {
	m, dir := selModel(t)
	dst := filepath.Join(dir, "sub")
	if err := os.Mkdir(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	m.reload()

	m.cur().cursorTo("a.txt")
	m = press(t, m, keyRune(' '), keyRune(' ')) // mark a.txt, b.txt
	m = press(t, m, keyRune('y'))
	if m.clip == nil || len(m.clip.paths) != 2 {
		t.Fatalf("yank should hold 2 paths, got %+v", m.clip)
	}
	if n := len(m.cur().selectedPaths()); n != 0 {
		t.Fatalf("yank should consume the selection, got %d", n)
	}

	m.cur().cursorTo("sub")
	m = pressCopy(t, m, keyRune('l'), keyRune('p'))
	for _, n := range []string{"a.txt", "b.txt"} {
		if _, err := os.Lstat(filepath.Join(dst, n)); err != nil {
			t.Errorf("%s was not pasted: %v", n, err)
		}
		if _, err := os.Lstat(filepath.Join(dir, n)); err != nil {
			t.Errorf("yank must not remove the source %s: %v", n, err)
		}
	}
}

// TestBulkCutPaste checks a multi-entry cut (x) pastes every path into the
// other directory, removes the sources, and empties the clipboard — the async
// cut-paste path, which trims the clipboard differently from a copy.
func TestBulkCutPaste(t *testing.T) {
	m, dir := selModel(t)
	dst := filepath.Join(dir, "sub")
	if err := os.Mkdir(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	m.reload()

	m.cur().cursorTo("a.txt")
	m = press(t, m, keyRune(' '), keyRune(' ')) // mark a.txt, b.txt
	m = press(t, m, keyRune('x'))
	if m.clip == nil || !m.clip.cut || len(m.clip.paths) != 2 {
		t.Fatalf("cut should hold 2 paths, got %+v", m.clip)
	}

	m.cur().cursorTo("sub")
	m = pressCopy(t, m, keyRune('l'), keyRune('p'))
	for _, n := range []string{"a.txt", "b.txt"} {
		if _, err := os.Lstat(filepath.Join(dst, n)); err != nil {
			t.Errorf("%s was not pasted: %v", n, err)
		}
		if _, err := os.Lstat(filepath.Join(dir, n)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("cut must remove the source %s, got %v", n, err)
		}
	}
	if m.clip != nil {
		t.Errorf("clip = %+v, want nil after a completed cut paste", m.clip)
	}
}

// TestBulkTransferMoveToOtherPane checks F6 moves the whole selection into the
// inactive pane's directory, removing the sources.
func TestBulkTransferMoveToOtherPane(t *testing.T) {
	m, dir := selModel(t)
	dst := t.TempDir()
	m.panes[1].dir = dst
	m.panes[1].reload("")

	m = pressCopy(t, m, keyRune('j'), keyRune(' '), keyRune(' ')) // mark a.txt, b.txt
	m = pressCopy(t, m, tea.KeyMsg{Type: tea.KeyF6})

	for _, n := range []string{"a.txt", "b.txt"} {
		if _, err := os.Lstat(filepath.Join(dst, n)); err != nil {
			t.Errorf("%s was not moved to the other pane: %v", n, err)
		}
		if _, err := os.Lstat(filepath.Join(dir, n)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("F6 must remove the source %s, got %v", n, err)
		}
	}
}

// TestBulkTransferToOtherPane checks F5 copies the whole selection into the
// inactive pane's directory.
func TestBulkTransferToOtherPane(t *testing.T) {
	m, dir := selModel(t)
	dst := t.TempDir()
	m.panes[1].dir = dst
	m.panes[1].reload("")

	m = pressCopy(t, m, keyRune('j'), keyRune(' '), keyRune(' ')) // mark a.txt, b.txt
	m = pressCopy(t, m, tea.KeyMsg{Type: tea.KeyF5})

	for _, n := range []string{"a.txt", "b.txt"} {
		if _, err := os.Lstat(filepath.Join(dst, n)); err != nil {
			t.Errorf("%s was not copied to the other pane: %v", n, err)
		}
		if _, err := os.Lstat(filepath.Join(dir, n)); err != nil {
			t.Errorf("F5 must leave the source %s in place: %v", n, err)
		}
	}
	if n := len(m.cur().selectedPaths()); n != 0 {
		t.Fatalf("transfer should consume the selection, got %d", n)
	}
}
