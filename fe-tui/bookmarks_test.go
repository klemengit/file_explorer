package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestBookmarkDeleteKey drives a real ctrl+d KeyMsg through the bookmarks picker
// and checks the bookmark is removed — guards against the key-string separator
// (ctrl+d vs ctrl-d) regressing.
func TestBookmarkDeleteKey(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if _, err := addBookmark("/tmp/aaa"); err != nil {
		t.Fatal(err)
	}
	if _, err := addBookmark("/tmp/bbb"); err != nil {
		t.Fatal(err)
	}

	m := newModel(t.TempDir())
	m.width, m.height = 80, 24
	tm, _ := m.openPicker(pickBookmarks)
	m = tm.(model)
	if m.mode != modePicker {
		t.Fatalf("expected picker mode, got %v", m.mode)
	}

	// ctrl+d should delete the bookmark under the cursor (the first one).
	tm, _ = m.updatePicker(tea.KeyMsg{Type: tea.KeyCtrlD})
	m = tm.(model)

	got := loadBookmarks()
	if len(got) != 1 || got[0] != "/tmp/bbb" {
		t.Fatalf("after ctrl+d want [/tmp/bbb], got %v", got)
	}
}
