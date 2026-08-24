package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// touchDir moves a directory's mtime by delta. Creating a file already moves
// it, but by however much the filesystem's timestamp resolution happens to be,
// and a test that wants to know the poll noticed should not also be a test of
// how fine tmpfs keeps time. Moving the stamp by hand makes "this directory
// changed" and "this directory did not" both exact.
func touchDir(t *testing.T, dir string, delta time.Duration) {
	t.Helper()
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	when := info.ModTime().Add(delta)
	if err := os.Chtimes(dir, when, when); err != nil {
		t.Fatal(err)
	}
}

// poll sends one refresh tick through Update.
func poll(t *testing.T, m model) model {
	t.Helper()
	tm, cmd := m.Update(refreshTickMsg{})
	if cmd == nil {
		t.Fatal("the poll did not schedule its next tick")
	}
	return tm.(model)
}

func hasRow(m model, name string) bool {
	for _, r := range m.cur().rows {
		if r.name == name {
			return true
		}
	}
	return false
}

// The whole point: a file that appears while fe is sitting in the directory
// shows up on its own, without leaving and coming back.
func TestPollPicksUpANewFile(t *testing.T) {
	m, dir := selModel(t)
	if hasRow(m, "aa.txt") {
		t.Fatal("aa.txt was there before the test made it")
	}

	if err := os.WriteFile(filepath.Join(dir, "aa.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	touchDir(t, dir, time.Second)

	m = poll(t, m)

	if !hasRow(m, "aa.txt") {
		t.Fatal("the poll did not pick up aa.txt")
	}
}

// A new entry sorting above the cursor must not drag the selection down with
// it. The cursor follows the file it was on, not the row number.
func TestPollKeepsTheCursorOnItsEntry(t *testing.T) {
	m, dir := selModel(t) // rows: .., a.txt, b.txt, c.txt
	m = press(t, m, keyRune('j'), keyRune('j'))
	if got := cursorName(m); got != "b.txt" {
		t.Fatalf("setup left the cursor on %q, want b.txt", got)
	}

	if err := os.WriteFile(filepath.Join(dir, "aa.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	touchDir(t, dir, time.Second)

	m = poll(t, m)

	if got := cursorName(m); got != "b.txt" {
		t.Fatalf("the poll moved the cursor to %q, want b.txt", got)
	}
}

// The entry under the cursor being the one that vanished is the case with no
// right answer to go back to, so the row number stands and the cursor lands on
// whatever moved up into the gap.
func TestPollHandlesTheCursorEntryVanishing(t *testing.T) {
	m, dir := selModel(t)
	m = press(t, m, keyRune('j'), keyRune('j'))
	if got := cursorName(m); got != "b.txt" {
		t.Fatalf("setup left the cursor on %q, want b.txt", got)
	}

	if err := os.Remove(filepath.Join(dir, "b.txt")); err != nil {
		t.Fatal(err)
	}
	touchDir(t, dir, time.Second)

	m = poll(t, m)

	if got := cursorName(m); got != "c.txt" {
		t.Fatalf("after b.txt went away the cursor sat on %q, want c.txt", got)
	}
}

// An unchanged directory is not re-read at all — that is what makes the poll
// cheap enough to run every second. Here the directory really did change but
// its mtime was put back, which is the only way to tell a listing that was
// skipped from one that was re-read and happened to match.
func TestPollSkipsAnUnchangedDirectory(t *testing.T) {
	m, dir := selModel(t)
	before, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "aa.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(dir, before.ModTime(), before.ModTime()); err != nil {
		t.Fatal(err)
	}

	m = poll(t, m)

	if hasRow(m, "aa.txt") {
		t.Fatal("the poll re-read a directory whose mtime had not moved")
	}
}

// The slow beat is the backstop for everything an mtime does not report — a
// file being written to in place. Every fullEvery-th tick reads regardless.
func TestPollReadsAnywayOnTheSlowBeat(t *testing.T) {
	m, dir := selModel(t)
	before, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "aa.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(dir, before.ModTime(), before.ModTime()); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < fullEvery; i++ {
		m = poll(t, m)
	}

	if !hasRow(m, "aa.txt") {
		t.Fatalf("%d ticks went by without a full re-read", fullEvery)
	}
}

// A live visual range is anchored to a row number, so the poll leaves the pane
// alone until the range is committed or cancelled.
func TestPollLeavesALiveVisualRangeAlone(t *testing.T) {
	m, dir := selModel(t)
	m = press(t, m, keyRune('j'), keyRune('V'), keyRune('j'))
	if !m.cur().visual {
		t.Fatal("setup did not start a visual range")
	}
	lo, hi, _ := m.cur().visualRange()

	if err := os.WriteFile(filepath.Join(dir, "aa.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	touchDir(t, dir, time.Second)

	m = poll(t, m)

	if hasRow(m, "aa.txt") {
		t.Fatal("the poll re-read a pane holding a live visual range")
	}
	if l, h, _ := m.cur().visualRange(); l != lo || h != hi {
		t.Fatalf("the visual range moved from %d-%d to %d-%d", lo, hi, l, h)
	}
}

// Everything that is not the browse view is holding something that names rows
// — a confirm's list of paths above all — so the poll stays out of it.
func TestPollStaysOutOfTheOtherModes(t *testing.T) {
	m, dir := selModel(t)
	m = press(t, m, keyRune('j'), keyRune('d')) // delete asks first
	if m.mode != modeConfirm {
		t.Fatalf("setup did not open the confirm (mode %d)", m.mode)
	}

	if err := os.WriteFile(filepath.Join(dir, "aa.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	touchDir(t, dir, time.Second)

	for i := 0; i <= fullEvery; i++ {
		m = poll(t, m)
	}

	if hasRow(m, "aa.txt") {
		t.Fatal("the poll re-read the listing under an open confirm")
	}
}

// ctrl-r is the same refresh on demand, for the times you would rather not
// wait the second out.
func TestCtrlRReloads(t *testing.T) {
	m, dir := selModel(t)
	before, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "aa.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(dir, before.ModTime(), before.ModTime()); err != nil {
		t.Fatal(err)
	}

	m = press(t, m, tea.KeyMsg{Type: tea.KeyCtrlR})

	if !hasRow(m, "aa.txt") {
		t.Fatal("ctrl-r did not re-read the pane")
	}
}

// Switching panes looks at a pane again, which is as good a moment as any to
// check it still says what the directory says.
func TestTabRefreshesBothPanes(t *testing.T) {
	m, dir := selModel(t)
	before, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "aa.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(dir, before.ModTime(), before.ModTime()); err != nil {
		t.Fatal(err)
	}

	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})

	if m.active != 1 {
		t.Fatalf("tab left the active pane at %d, want 1", m.active)
	}
	for i := range m.panes {
		found := false
		for _, r := range m.panes[i].rows {
			if r.name == "aa.txt" {
				found = true
			}
		}
		if !found {
			t.Fatalf("pane %d was not re-read by tab", i)
		}
	}
}
