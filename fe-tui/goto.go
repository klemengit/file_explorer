package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// gotoTarget is one `g`-chord destination: press g, then key, and the active
// pane jumps to path. Three of them are resolved at the moment the chord is
// pressed rather than up front (the top of the list, the directory fe started
// in, the other pane's directory) and carry an empty path; they live in the
// list anyway so the hint line and the help window can offer them alongside
// the rest.
type gotoTarget struct {
	key   string // the second key of the chord — always a single rune
	label string // what the hint line and help call it
	path  string // absolute destination, or "" for the resolved-on-press ones
}

// configHome is ${XDG_CONFIG_HOME:-~/.config}.
func configHome() string {
	if base := os.Getenv("XDG_CONFIG_HOME"); base != "" {
		return base
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config")
}

// gotoTargets builds the chord list for this machine: the built-in set, with
// the user's own entries merged over it, cut down to the directories that
// actually exist.
func gotoTargets() []gotoTarget {
	home, _ := os.UserHomeDir()

	var userDirs map[string]string
	if b, err := os.ReadFile(filepath.Join(configHome(), "user-dirs.dirs")); err == nil {
		userDirs = parseUserDirs(string(b), home)
	}
	ts := defaultGotos(home, userDirs)

	if b, err := os.ReadFile(gotoConfigPath()); err == nil {
		ts = mergeGotos(ts, parseGotoConfig(string(b), home))
	}
	return keepExisting(ts)
}

// gotoConfigPath is ${XDG_CONFIG_HOME:-~/.config}/fe/goto.
func gotoConfigPath() string { return filepath.Join(configHome(), "fe", "goto") }

// defaultGotos is the built-in chord set, in the order it is offered. The
// user-directory paths come from XDG where the system provides them, so a
// localized setup jumps to (and shows) ~/Prenosi rather than a ~/Downloads
// that isn't there.
func defaultGotos(home string, userDirs map[string]string) []gotoTarget {
	xdg := func(key, fallback string) string {
		if p := userDirs[key]; p != "" {
			return p
		}
		if home == "" {
			return ""
		}
		return filepath.Join(home, fallback)
	}
	under := func(parts ...string) string {
		if home == "" {
			return ""
		}
		return filepath.Join(append([]string{home}, parts...)...)
	}
	return []gotoTarget{
		{key: "g", label: "top"},
		{key: "h", label: "~", path: home},
		{key: "d", path: xdg("XDG_DOWNLOAD_DIR", "Downloads")},
		{key: "D", path: xdg("XDG_DOCUMENTS_DIR", "Documents")},
		{key: "k", path: xdg("XDG_DESKTOP_DIR", "Desktop")},
		{key: "p", path: xdg("XDG_PICTURES_DIR", "Pictures")},
		{key: "m", path: xdg("XDG_MUSIC_DIR", "Music")},
		{key: "v", path: xdg("XDG_VIDEOS_DIR", "Videos")},
		{key: "c", path: under(".config")},
		{key: "t", path: "/tmp"},
		{key: "r", label: "/", path: "/"},
		{key: ".", label: "start dir"},
		{key: "o", label: "other pane"},
	}
}

// parseUserDirs reads the XDG user-dirs file, whose lines look like
//
//	XDG_DOWNLOAD_DIR="$HOME/Downloads"
//
// and returns the paths by their XDG key, with $HOME expanded.
func parseUserDirs(content, home string) map[string]string {
	out := make(map[string]string)
	sc := bufio.NewScanner(strings.NewReader(content))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(key), "export "))
		val = strings.Trim(strings.TrimSpace(val), `"'`)
		if key == "" || val == "" {
			continue
		}
		if home != "" {
			val = strings.ReplaceAll(val, "${HOME}", home)
			val = strings.ReplaceAll(val, "$HOME", home)
		}
		if strings.HasPrefix(val, "$") {
			continue // some other variable we can't expand
		}
		out[key] = filepath.Clean(val)
	}
	return out
}

// parseGotoConfig reads the user's own chords, one per line:
//
//	w  ~/work/current      # key, then the path it goes to
//	d = /mnt/big/downloads # an = between them is allowed
//
// Blank lines and # comments are ignored, as is any line whose key isn't a
// single character.
func parseGotoConfig(content, home string) []gotoTarget {
	var out []gotoTarget
	sc := bufio.NewScanner(strings.NewReader(content))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, rest, ok := strings.Cut(line, " ")
		if !ok {
			if key, rest, ok = strings.Cut(line, "\t"); !ok {
				continue
			}
		}
		path := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(rest), "="))
		if key == "" || path == "" || len([]rune(key)) != 1 {
			continue
		}
		out = append(out, gotoTarget{key: key, path: expandHome(path, home)})
	}
	return out
}

// expandHome turns a leading ~ into the home directory.
func expandHome(p, home string) string {
	if home == "" || p != "~" && !strings.HasPrefix(p, "~/") {
		return p
	}
	return filepath.Join(home, strings.TrimPrefix(p, "~"))
}

// mergeGotos lays the user's entries over the built-in ones: a key that
// already exists is replaced where it stands, so the order stays predictable,
// and anything new is appended.
func mergeGotos(base, user []gotoTarget) []gotoTarget {
	out := append([]gotoTarget(nil), base...)
	for _, u := range user {
		replaced := false
		for i := range out {
			if out[i].key == u.key {
				out[i] = u
				replaced = true
				break
			}
		}
		if !replaced {
			out = append(out, u)
		}
	}
	return out
}

// isDynamicGoto reports whether a chord is resolved when it is pressed rather
// than from a fixed path, and so exists whatever the filesystem looks like.
func isDynamicGoto(key string) bool {
	switch key {
	case "g", ".", "o":
		return true
	}
	return false
}

// keepExisting drops the chords that would be no use — a directory that isn't
// there (an unpopulated ~/Music shouldn't take up a key), or one an earlier
// chord already covers — and gives the survivors a label.
//
// The duplicate rule matters more than it sounds: a desktop that has had its
// Desktop folder turned off writes XDG_DESKTOP_DIR="$HOME/", which would
// otherwise leave gk as a second, confusingly-labelled gh.
func keepExisting(ts []gotoTarget) []gotoTarget {
	out := make([]gotoTarget, 0, len(ts))
	seen := make(map[string]bool, len(ts))
	for _, t := range ts {
		if t.path == "" {
			if !isDynamicGoto(t.key) {
				continue // nothing to go to (no home directory, say)
			}
			out = append(out, t)
			continue
		}
		t.path = filepath.Clean(t.path)
		info, err := os.Stat(t.path)
		if err != nil || !info.IsDir() || seen[t.path] {
			continue
		}
		seen[t.path] = true
		if t.label == "" {
			t.label = filepath.Base(t.path)
		}
		out = append(out, t)
	}
	return out
}

// gotoFor looks up the chord bound to key. Only real destinations match; the
// resolved-on-press entries are handled by the caller.
func (m model) gotoFor(key string) (gotoTarget, bool) {
	for _, t := range m.gotos {
		if t.key == key && t.path != "" {
			return t, true
		}
	}
	return gotoTarget{}, false
}

// gotoEntries presents this machine's destinations as chord continuations, so
// the footer hint and the which-key window can describe goto without knowing
// anything about directories.
func gotoEntries(m model) []chordEntry {
	out := make([]chordEntry, 0, len(m.gotos))
	for _, t := range m.gotos {
		out = append(out, chordEntry{key: t.key, label: t.label})
	}
	return out
}

// gotoHint is the one-line reminder shown in the footer while a g is pending.
func (m model) gotoHint(width int) string {
	return chordHint("goto", gotoEntries(m), width)
}

// gotoChord handles the key pressed after g. It always consumes that key, so
// `gd` is "go to Downloads" and never "delete".
func (m model) gotoChord(key string) (tea.Model, tea.Cmd) {
	// gg keeps its meaning whatever the config says; everything else can be
	// bound to a directory of your own.
	switch key {
	case "esc":
		return m, nil // chord abandoned
	case "g":
		p := m.cur()
		p.cursor, p.top = 0, 0
		return m, nil
	}
	if t, ok := m.gotoFor(key); ok {
		m.gotoDir(t.path)
		return m, nil
	}
	switch key {
	case ".":
		m.gotoDir(m.startDir)
		return m, nil
	case "o":
		m.gotoDir(m.other().dir)
		return m, nil
	}
	if len([]rune(key)) != 1 {
		return m, nil // an arrow, tab, a function key: just call the chord off
	}
	m.setStatus(lvlWarn, "No goto bound to '%s' — ? lists them", key)
	return m, nil
}

// gotoDir jumps the active pane to dir, reporting a destination that has gone
// away rather than leaving the pane on an empty listing.
func (m *model) gotoDir(dir string) {
	if dir == "" {
		return
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		m.setStatus(lvlErr, "Cannot go to %s", abbrevHome(dir))
		return
	}
	m.enterDir(dir)
	m.setStatus(lvlInfo, "→ %s", abbrevHome(dir))
}
