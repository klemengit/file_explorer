package main

import (
	"os"
	"path/filepath"
	"strings"
)

// The right pane is remembered between runs: the left pane always opens where
// fe was started, the right one comes back to wherever it was left. The path
// lives in ${XDG_STATE_HOME:-~/.local/state}/fe/right-pane — state rather than
// data, since it is this machine's last position and not something worth
// keeping (bookmarks, which are, stay in the data directory).
func statePath() string {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return ""
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "fe", "right-pane")
}

// loadRightPane returns the remembered right-pane directory, or "" when there
// is none or it no longer exists.
func loadRightPane() string {
	p := statePath()
	if p == "" {
		return ""
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	dir := strings.TrimSpace(string(b))
	if i := strings.IndexByte(dir, '\n'); i >= 0 {
		dir = strings.TrimSpace(dir[:i])
	}
	if dir == "" {
		return ""
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return "" // it was deleted, or lived on a drive that is now unplugged
	}
	return dir
}

// saveRightPane records where the right pane was left.
func saveRightPane(dir string) error {
	p := statePath()
	if p == "" || dir == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(dir+"\n"), 0o644)
}

// saveSession is called on the way out of the program. A failure here is not
// worth a message on a screen that is about to disappear.
func (m model) saveSession() {
	_ = saveRightPane(m.panes[1].dir)
}
