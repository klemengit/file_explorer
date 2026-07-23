package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func TestAbbrevHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home dir")
	}
	if got := abbrevHome(home); got != "~" {
		t.Errorf("abbrevHome(home) = %q, want ~", got)
	}
	p := filepath.Join(home, "code", "proj")
	if got, want := abbrevHome(p), "~/code/proj"; got != want {
		t.Errorf("abbrevHome(%q) = %q, want %q", p, got, want)
	}
	if got := abbrevHome("/etc/hosts"); got != "/etc/hosts" {
		t.Errorf("non-home path changed: %q", got)
	}
	if got := abbrevHome("relative/path"); got != "relative/path" {
		t.Errorf("relative path changed: %q", got)
	}
}

// ansiRe matches SGR escape sequences; named to leave "ansi" free for the
// charmbracelet/x/ansi package imported elsewhere in the package.
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func strip(s string) string { return ansiRe.ReplaceAllString(s, "") }

func TestHumanSize(t *testing.T) {
	cases := map[int64]string{0: "0B", 512: "512B", 1500: "1.5K", 4200000: "4.0M"}
	for in, want := range cases {
		if got := humanSize(in); got != want {
			t.Errorf("humanSize(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestRenderRowColumns(t *testing.T) {
	p := pane{width: 90}
	mt := time.Date(2026, 1, 2, 15, 4, 0, 0, time.UTC)

	file := row{label: "alpha.txt", name: "alpha.txt", size: 1500, modTime: mt}
	out := strip(p.renderRow(file, false, false))
	if !strings.Contains(out, "1.5K") {
		t.Errorf("file row missing size: %q", out)
	}
	if !strings.Contains(out, "2026-01-02 15:04") {
		t.Errorf("file row missing date: %q", out)
	}

	dir := row{label: "sub/", name: "sub", isDir: true, modTime: mt}
	out = strip(p.renderRow(dir, false, false))
	if !strings.Contains(out, "-") {
		t.Errorf("dir row should show '-' for size: %q", out)
	}

	parent := row{label: "..", isParent: true}
	out = strip(p.renderRow(parent, true, false))
	if strings.Contains(out, "2026") {
		t.Errorf("parent row should have no date: %q", out)
	}

	// cursor row still carries both columns
	out = strip(p.renderRow(file, true, false))
	if !strings.Contains(out, "1.5K") || !strings.Contains(out, "2026-01-02 15:04") {
		t.Errorf("cursor file row missing columns: %q", out)
	}
}

// Emoji and CJK characters draw two columns each. Measuring them as one rune
// makes a row overflow its pane, and lipgloss wraps the overflow onto a second
// line instead of clipping it — which grows one pane's box taller than the
// other and skews the whole two-pane layout.
func TestRenderRowWideRunesStayInsideThePane(t *testing.T) {
	p := pane{width: 60}
	mt := time.Date(2026, 1, 2, 15, 4, 0, 0, time.UTC)

	for _, name := range []string{
		"⚡ Dashboard.md",
		"📒Lists.md",
		"✅ TODO.md",
		"🔗 Links.md",
		"日本語のファイル.md",
		strings.Repeat("🔗", 40) + ".md", // long enough to need truncating
	} {
		r := row{label: name, name: name, size: 1500, modTime: mt}
		for _, cursor := range []bool{false, true} {
			out := strip(p.renderRow(r, cursor, false))
			if w := ansi.StringWidth(out); w > p.width {
				t.Errorf("row %q (cursor=%v) drew %d cells, pane is %d: %q", name, cursor, w, p.width, out)
			}
			if strings.Contains(out, "\n") {
				t.Errorf("row %q wrapped: %q", name, out)
			}
		}
	}
}

// The same overflow reaches the pane box, where a wrapped row shows up as an
// extra line and pushes this pane's border out of step with its neighbour's.
func TestPaneBoxHeightSurvivesWideRunes(t *testing.T) {
	mt := time.Date(2026, 1, 2, 15, 4, 0, 0, time.UTC)
	m := model{width: 120, height: 24}
	m.panes[0] = pane{dir: "/tmp", width: 58, height: 23}
	m.panes[1] = m.panes[0]
	for _, name := range []string{"⚡ Dashboard.md", "📒Lists.md", "✅ TODO.md"} {
		m.panes[1].rows = append(m.panes[1].rows, row{label: name, name: name, size: 1500, modTime: mt})
		m.panes[0].rows = append(m.panes[0].rows, row{label: "plain.md", name: "plain.md", size: 1500, modTime: mt})
	}

	left := strings.Count(m.paneBox(&m.panes[0], true), "\n")
	right := strings.Count(m.paneBox(&m.panes[1], false), "\n")
	if left != right {
		t.Errorf("emoji pane is %d lines tall, plain pane is %d — the boxes will not line up", right+1, left+1)
	}
}

// TestRenderRowSelectionGutter checks the two-cell gutter encodes cursor and
// selection independently, and that neither steals width from the columns.
func TestRenderRowSelectionGutter(t *testing.T) {
	p := pane{width: 90}
	file := row{label: "alpha.txt", name: "alpha.txt", size: 1500}

	cases := []struct {
		cursor, marked bool
		want           string
	}{
		{false, false, "  alpha.txt"},
		{false, true, " *alpha.txt"},
		{true, false, "▶ alpha.txt"},
		{true, true, "▶*alpha.txt"},
	}
	for _, c := range cases {
		out := strip(p.renderRow(file, c.cursor, c.marked))
		if !strings.HasPrefix(out, c.want) {
			t.Errorf("renderRow(cursor=%v, marked=%v) = %q, want prefix %q", c.cursor, c.marked, out, c.want)
		}
	}
}
