package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// writeAt creates name inside dir with the given modification time.
func writeAt(t *testing.T, dir, name string, mt time.Time) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, mt, mt); err != nil {
		t.Fatal(err)
	}
}

// TestListDirTimeSortFlat checks that sorting by mtime is a flat, newest-first
// order over the whole directory — a directory that is older than a file must
// still sort below that newer file (no dirs-first grouping).
func TestListDirTimeSortFlat(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

	// old directory, then newer files
	sub := filepath.Join(dir, "olddir")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	os.Chtimes(sub, base, base)
	writeAt(t, dir, "mid.txt", base.Add(time.Hour))
	writeAt(t, dir, "newest.txt", base.Add(2*time.Hour))

	names := func(sm sortMode) []string {
		entries, err := listDir(dir, sm, true, false)
		if err != nil {
			t.Fatal(err)
		}
		out := make([]string, len(entries))
		for i, e := range entries {
			out[i] = e.name
		}
		return out
	}

	if got, want := names(sortTimeDesc), []string{"newest.txt", "mid.txt", "olddir"}; !equalStrs(got, want) {
		t.Fatalf("newest-first: want %v, got %v", want, got)
	}
	if got, want := names(sortTimeAsc), []string{"olddir", "mid.txt", "newest.txt"}; !equalStrs(got, want) {
		t.Fatalf("oldest-first: want %v, got %v", want, got)
	}
}

func equalStrs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestListDirNameSortGroups checks that name sorting still groups dirs before
// files (original fe.sh behaviour), independent of mtime.
func TestListDirNameSortGroups(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Mkdir(filepath.Join(dir, "zdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeAt(t, dir, "afile.txt", base)

	entries, err := listDir(dir, sortName, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || !entries[0].isDir || entries[0].name != "zdir" {
		t.Fatalf("name sort should list dir 'zdir' first, got %+v", entries)
	}
}

// TestNKeyCycles drives the `n` key three times and asserts the sort cycles
// newest-first → oldest-first → original name order, staying in browse mode.
func TestNKeyCycles(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	writeAt(t, dir, "a.txt", base)                // oldest
	writeAt(t, dir, "b.txt", base.Add(time.Hour)) // newest

	pressN := func(m model) model {
		tm, _ := m.updateBrowse(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
		mm := tm.(model)
		if mm.mode != modeBrowse {
			t.Fatalf("n should stay in browse mode, got %v", mm.mode)
		}
		return mm
	}

	m := newModel(dir)
	m.width, m.height = 80, 24
	m.layout()

	// 1st press: newest first — rows[0] is "..", rows[1] the newest file.
	m = pressN(m)
	if p := m.cur(); p.sortMode != sortTimeDesc || p.rows[1].name != "b.txt" {
		t.Fatalf("1st n: want newest-first with b.txt on top, got mode=%v rows=%+v", p.sortMode, p.rows)
	}
	// 2nd press: oldest first.
	m = pressN(m)
	if p := m.cur(); p.sortMode != sortTimeAsc || p.rows[1].name != "a.txt" {
		t.Fatalf("2nd n: want oldest-first with a.txt on top, got mode=%v rows=%+v", p.sortMode, p.rows)
	}
	// 3rd press: back to original name order.
	m = pressN(m)
	if p := m.cur(); p.sortMode != sortName {
		t.Fatalf("3rd n: want original name sort, got mode=%v", p.sortMode)
	}
}
