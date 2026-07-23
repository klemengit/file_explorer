package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// pressAll drives keys through the top-level Update, so a sequence can cross
// modes. press cannot: it always calls the browse handler, so the key after
// the one that opens a prompt would be read as another browse command.
func pressAll(t *testing.T, m model, keys ...rune) model {
	t.Helper()
	for _, r := range keys {
		tm, _ := m.Update(keyRune(r))
		m = tm.(model)
	}
	return m
}

// promptModel is a browsable model sized like a real terminal, so the prompt
// windows have somewhere to float.
func promptModel(t *testing.T) (model, string) {
	t.Helper()
	m, dir := selModel(t)
	m.width, m.height = 100, 24
	m.layout()
	return m, dir
}

func TestPromptsAreWindowsNotFooterLines(t *testing.T) {
	m, _ := promptModel(t)

	cases := []struct {
		keys  []rune
		title string
	}{
		{[]rune{'/'}, "filter"},
		{[]rune{'j', 'r'}, "rename"},
		{[]rune{'a'}, "new"},
		{[]rune{'j', 'd'}, "confirm"},
	}
	for _, c := range cases {
		mm := pressAll(t, m, c.keys...)
		if !mm.promptModal() {
			t.Errorf("%q did not open a prompt window (mode %d)", string(c.keys), mm.mode)
			continue
		}
		out := strip(mm.View())
		if !strings.Contains(out, c.title) {
			t.Errorf("%q: window is not titled %q:\n%s", string(c.keys), c.title, out)
		}
		// The panes stay visible around it — that is what makes it a float
		// rather than a screen.
		if !strings.Contains(out, "a.txt") {
			t.Errorf("%q: the panes vanished behind the prompt:\n%s", string(c.keys), out)
		}
		// And the old footer prompt is gone.
		if f := strip(mm.footer()); strings.Contains(f, "(y/n)") {
			t.Errorf("%q: footer still carries the prompt: %q", string(c.keys), f)
		}
	}
}

func TestPromptBoxIsRectangularAndFits(t *testing.T) {
	m, _ := promptModel(t)
	for _, keys := range [][]rune{{'/'}, {'j', 'r'}, {'a'}, {'j', 'd'}} {
		mm := pressAll(t, m, keys...)
		box := mm.promptBox()
		assertRectangular(t, box)
		if w := lipgloss.Width(box); w > mm.width {
			t.Errorf("%q: window is %d wide, terminal is %d", string(keys), w, mm.width)
		}
		// Floating it must not change the size of the frame.
		view := mm.View()
		if h := lipgloss.Height(view); h != mm.height {
			t.Errorf("%q: view is %d lines, terminal is %d", string(keys), h, mm.height)
		}
		for i, l := range strings.Split(view, "\n") {
			if w := ansi.StringWidth(strip(l)); w > mm.width {
				t.Errorf("%q: line %d is %d cells, terminal is %d", string(keys), i, w, mm.width)
			}
		}
	}
}

// The rename window says which name is being replaced, and the new window
// says where the entry will land.
func TestPromptWindowNamesWhatItIsActingOn(t *testing.T) {
	m, dir := promptModel(t)

	r := pressAll(t, m, 'j', 'r')
	if out := strip(r.View()); !strings.Contains(out, "renaming 'a.txt'") {
		t.Errorf("rename window does not name its target:\n%s", out)
	}

	a := pressAll(t, m, 'a')
	if got, want := a.promptSubject, "in "+abbrevHome(dir); got != want {
		t.Errorf("new window subject = %q, want %q", got, want)
	}
	// Temp-dir paths are long, so what reaches the window is the tail of it.
	if out := strip(a.View()); !strings.Contains(out, filepath.Base(dir)) {
		t.Errorf("new window does not say where the entry lands:\n%s", out)
	}
}

// A deep path must not stretch the window across the terminal; it gets cut
// from the left instead, which keeps the part that identifies the directory.
func TestPromptContextDoesNotStretchTheWindow(t *testing.T) {
	m, _ := promptModel(t)
	m.mode = modeCreate
	m.promptSubject = "in /a/very/deeply/nested/path/that/goes/on/and/on/and/on/for/ages/indeed"

	box := m.promptBox()
	if w := lipgloss.Width(box); w > promptMaxW+2 {
		t.Errorf("window grew to %d columns for a long path, cap is %d", w, promptMaxW+2)
	}
	if out := strip(box); !strings.Contains(out, "ages/indeed") {
		t.Errorf("the tail of the path is what identifies it:\n%s", out)
	}
	assertRectangular(t, box)
}

func TestTruncateLeftKeepsTheTail(t *testing.T) {
	if got := truncateLeft("abcdef", 10); got != "abcdef" {
		t.Errorf("short string changed: %q", got)
	}
	got := truncateLeft("/home/k/one/two/three", 10)
	if ansi.StringWidth(got) != 10 {
		t.Errorf("truncateLeft width = %d, want 10: %q", ansi.StringWidth(got), got)
	}
	if !strings.HasPrefix(got, "…") || !strings.HasSuffix(got, "three") {
		t.Errorf("want an elided head and the tail intact, got %q", got)
	}
	if got := truncateLeft("abc", 0); got != "" {
		t.Errorf("zero width = %q, want empty", got)
	}
}

// Filtering keeps working while its window is up: the panes behind it show the
// matches, which is the whole reason the filter is live.
func TestFilterWindowShowsMatchesBehindIt(t *testing.T) {
	m, _ := promptModel(t)
	m = pressAll(t, m, '/', 'a')

	out := strip(m.View())
	if !strings.Contains(out, "filter") {
		t.Fatalf("filter window missing:\n%s", out)
	}
	if !strings.Contains(out, "a.txt") {
		t.Errorf("the match is not visible behind the window:\n%s", out)
	}
	// The filter is per-pane, so only the active one narrows; the other keeps
	// its full listing.
	var names []string
	for _, r := range m.cur().rows {
		names = append(names, r.name)
	}
	if len(names) != 1 || names[0] != "a.txt" {
		t.Errorf("active pane rows = %v, want just a.txt", names)
	}
	if len(m.other().rows) <= 1 {
		t.Errorf("the inactive pane should be untouched, has %d rows", len(m.other().rows))
	}
}

// The footer drops the browse hints while a window is open — they don't apply —
// but a status line is still worth showing.
func TestFooterYieldsToThePromptWindow(t *testing.T) {
	m, _ := promptModel(t)
	m = pressAll(t, m, 'a')
	if got := strip(m.footer()); got != "" {
		t.Errorf("footer = %q, want empty while a prompt window is open", got)
	}
	m.setStatus(lvlWarn, "careful")
	if got := strip(m.footer()); !strings.Contains(got, "careful") {
		t.Errorf("footer = %q, want the status line", got)
	}
}
