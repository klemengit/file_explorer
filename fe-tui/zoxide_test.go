package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// Recorded `zoxide query -l` output: frecency order, one path per line, with
// the stray blank line a trailing newline leaves behind.
const zoxideFixture = `/home/kz/Data/Projekti/KolektorETRA
/home/kz/Data/git_repos
/home/kz/Data/git_repos/file_exp
/home/kz/.config/nvim
`

func TestParseZoxideListKeepsFrecencyOrder(t *testing.T) {
	got := parseZoxideList(zoxideFixture)
	want := []string{
		"/home/kz/Data/Projekti/KolektorETRA",
		"/home/kz/Data/git_repos",
		"/home/kz/Data/git_repos/file_exp",
		"/home/kz/.config/nvim",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d paths, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("path %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseZoxideListOnEmptyOutput(t *testing.T) {
	if got := parseZoxideList(""); len(got) != 0 {
		t.Errorf("empty output should give no paths, got %v", got)
	}
	if got := parseZoxideList("\n\n"); len(got) != 0 {
		t.Errorf("blank lines should give no paths, got %v", got)
	}
}

// A zoxide database accumulates directories that have since been deleted, and
// those are dropped rather than listed.
func TestExistingDirsDropsWhatIsGone(t *testing.T) {
	dir := t.TempDir()
	live := filepath.Join(dir, "live")
	if err := os.Mkdir(live, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "afile")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	got := existingDirs([]string{live, filepath.Join(dir, "gone"), file})
	if len(got) != 1 || got[0] != live {
		t.Fatalf("want just %q, got %v", live, got)
	}
}

// The rule that separates Z from fe's ordinary fuzzy filter: every term in
// order, and the last term inside the final path component.
func TestZoxideMatch(t *testing.T) {
	cases := []struct {
		query, path string
		want        bool
	}{
		{"", "/home/kz/anything", true},
		{"   ", "/home/kz/anything", true},

		// The last term has to land in the final component.
		{"git", "/home/kz/Data/git_repos", true},
		{"git", "/home/kz/Data/git_repos/file_exp", false},
		{"exp", "/home/kz/Data/git_repos/file_exp", true},

		// Several terms, matched in order along the path.
		{"data exp", "/home/kz/Data/git_repos/file_exp", true},
		{"exp data", "/home/kz/Data/git_repos/file_exp", false},
		{"data git", "/home/kz/Data/git_repos", true},

		// Case-insensitive both ways round.
		{"REPOS", "/home/kz/Data/git_repos", true},
		{"repos", "/home/kz/Data/GIT_REPOS", true},

		// Substrings, not subsequences: "dt" matches "Data" for fuzzyMatch
		// but must not here, or Z would offer half the database.
		{"dt", "/home/kz/Data", false},

		{"nvim", "/home/kz/.config/nvim", true},
		{"absent", "/home/kz/Data", false},
	}
	for _, c := range cases {
		if got := zoxideMatch(c.query, c.path); got != c.want {
			t.Errorf("zoxideMatch(%q, %q) = %v, want %v", c.query, c.path, got, c.want)
		}
	}
}

// zoxideMatch must not panic on the paths that have no separator to find.
func TestZoxideMatchOddPaths(t *testing.T) {
	for _, p := range []string{"/", "", "relative"} {
		zoxideMatch("x", p)
	}
}

// The picker routes the zoxide list through zoxide's rule and every other list
// through fe's, which is the difference typing into the Z window depends on.
func TestPickerMatchPicksTheRightRule(t *testing.T) {
	m := newModel(t.TempDir())
	path := "/home/kz/Data/git_repos/file_exp"

	m.pickerKind = pickZoxide
	if m.pickerMatch("git", path) {
		t.Error("zoxide picker should reject a mid-path match for 'git'")
	}
	m.pickerKind = pickBookmarks
	if !m.pickerMatch("git", path) {
		t.Error("other pickers should keep fe's plain fuzzy match")
	}
}

// Z is registered, searchable in the palette, and gated on zoxide being
// installed with a reason rather than the generic "nothing to act on here".
func TestZoxideCommandIsRegistered(t *testing.T) {
	c, ok := commandFor("Z")
	if !ok {
		t.Fatal("Z is not bound to any command")
	}
	if c.when == nil {
		t.Error("Z should be gated on zoxide being installed")
	}
	if c.unavailable == "" {
		t.Error("Z should say why it is unavailable")
	}
	if !strings.Contains(c.alt+" "+c.desc, "zoxide") {
		t.Error("Z should be findable in the palette by the word zoxide")
	}
}

// Pressing Z does one of two things depending on the machine: opens the picker,
// or says why it can't. Never nothing at all — silence is what the unavailable
// message exists to prevent.
func TestPressingZ(t *testing.T) {
	m := newModel(t.TempDir())
	m.width, m.height = 80, 24

	c, _ := commandFor("Z")
	available := c.available(m)

	tm, _ := m.updateBrowse(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Z'}})
	got := tm.(model)

	if !available {
		if got.status != c.unavailable {
			t.Errorf("status = %q, want %q", got.status, c.unavailable)
		}
		if got.mode != modeBrowse {
			t.Errorf("mode = %v, want browse", got.mode)
		}
		return
	}
	// With zoxide installed, Z opens the picker — unless the database has
	// nothing left in it, which is its own reported outcome.
	if got.mode == modePicker {
		if got.pickerKind != pickZoxide {
			t.Errorf("picker kind = %v, want pickZoxide", got.pickerKind)
		}
		if !strings.Contains(got.pickerTitle, "zoxide") {
			t.Errorf("picker title = %q, want it to name zoxide", got.pickerTitle)
		}
		return
	}
	if got.status == "" {
		t.Error("Z did nothing and said nothing")
	}
}

// The zoxide picker is a floating window like bookmarks, not the full-screen
// one deep find uses.
func TestZoxidePickerFloats(t *testing.T) {
	m := newModel(t.TempDir())
	m.width, m.height = 80, 24
	m.pickerKind = pickZoxide
	if !m.pickerPopup() {
		t.Error("the zoxide picker should be a floating window")
	}
	if !strings.Contains(m.pickerHint(), "forget") {
		t.Errorf("hint = %q, want it to mention ctrl-d forget", m.pickerHint())
	}
}
