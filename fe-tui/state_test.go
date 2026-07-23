package main

import (
	"os"
	"path/filepath"
	"testing"
)

// stateDir points the remembered-right-pane file at a temp directory, so the
// tests never touch the real one.
func stateDir(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
}

func TestSaveAndLoadRightPane(t *testing.T) {
	stateDir(t)
	dir := t.TempDir()

	if got := loadRightPane(); got != "" {
		t.Errorf("with no state file, loadRightPane = %q, want empty", got)
	}
	if err := saveRightPane(dir); err != nil {
		t.Fatalf("saveRightPane: %v", err)
	}
	if got := loadRightPane(); got != dir {
		t.Errorf("loadRightPane = %q, want %q", got, dir)
	}
}

func TestLoadRightPaneIgnoresDirectoriesThatAreGone(t *testing.T) {
	stateDir(t)
	gone := filepath.Join(t.TempDir(), "unplugged")
	if err := os.Mkdir(gone, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := saveRightPane(gone); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}
	if got := loadRightPane(); got != "" {
		t.Errorf("loadRightPane = %q, want empty for a directory that no longer exists", got)
	}
}

// Quitting is the only moment the position is written, so both quit keys have
// to do it.
func TestQuitRemembersRightPane(t *testing.T) {
	stateDir(t)
	m, dir := selModel(t)
	right := filepath.Join(dir, "right")
	if err := os.Mkdir(right, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := m.panes[1].enterDir(right, ""); err != nil {
		t.Fatal(err)
	}

	m = press(t, m, keyRune('q'))
	if got := loadRightPane(); got != right {
		t.Errorf("after q, remembered %q, want %q", got, right)
	}
}
