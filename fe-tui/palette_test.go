package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// paletteModel is a browsable model with the palette already open.
func paletteModel(t *testing.T) (model, string) {
	t.Helper()
	m, dir := selModel(t)
	m.width, m.height = 100, 24
	m.layout()
	m, _ = send(t, m, keyRune(':'))
	if m.mode != modePalette {
		t.Fatalf(": did not open the palette, mode = %d", m.mode)
	}
	return m, dir
}

// typeQuery sends a string through the top-level Update one key at a time, so
// it reaches whichever window is open. (create_test's typeInto goes straight to
// the prompt handler, which the palette is not.)
func typeQuery(t *testing.T, m model, s string) model {
	t.Helper()
	for _, r := range s {
		m, _ = send(t, m, keyRune(r))
	}
	return m
}

func TestPaletteListsTheCommandsWithTheirKeys(t *testing.T) {
	m, _ := paletteModel(t)

	out := strip(m.View())
	if !strings.Contains(out, "commands") {
		t.Fatalf("palette window missing:\n%s", out)
	}
	// The point of the thing: the command and the key that runs it, together.
	if !strings.Contains(out, "switch active pane") || !strings.Contains(out, "tab") {
		t.Errorf("palette does not show commands with their keys:\n%s", out)
	}
	// The panes stay visible around it.
	if !strings.Contains(out, "a.txt") {
		t.Errorf("the panes vanished behind the palette:\n%s", out)
	}
}

// Motions are in the help but not in the palette — a fuzzy list is a slow way
// to press j.
func TestPaletteLeavesOutTheMotions(t *testing.T) {
	m, _ := paletteModel(t)
	for _, i := range m.paletteRows {
		if commandSet[i].hidden {
			t.Errorf("palette lists the hidden command %q", commandSet[i].desc)
		}
	}
	m = typeQuery(t, m, "half page")
	if len(m.paletteRows) != 0 {
		t.Errorf("searching for a motion found %d rows, want none", len(m.paletteRows))
	}
	if out := strip(m.paletteBox()); !strings.Contains(out, "no command matches") {
		t.Errorf("an empty result should say so:\n%s", out)
	}
}

// Typing narrows the list, including on the synonyms a command doesn't display.
func TestPaletteMatchesNamesAndSynonyms(t *testing.T) {
	cases := []struct{ query, want string }{
		{"rename", "rename"},
		{"mkdir", "new file (or folder, end with /)"}, // alt keyword
		{"archive", "zip / unzip"},                    // alt keyword
		{"drives", "external drives (u unmount, e eject)"},
	}
	for _, c := range cases {
		m, _ := paletteModel(t)
		m = typeQuery(t, m, c.query)
		if len(m.paletteRows) == 0 {
			t.Errorf("%q matched nothing", c.query)
			continue
		}
		var descs []string
		for _, i := range m.paletteRows {
			descs = append(descs, commandSet[i].desc)
		}
		found := false
		for _, d := range descs {
			if d == c.want {
				found = true
			}
		}
		if !found {
			t.Errorf("%q matched %v, want %q among them", c.query, descs, c.want)
		}
	}
}

// Running a command from the palette does what its key does.
func TestPaletteRunsTheSelectedCommand(t *testing.T) {
	m, _ := paletteModel(t)
	m = typeQuery(t, m, "dotfiles")
	before := m.cur().showDots

	m, _ = send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeBrowse {
		t.Errorf("running a command should close the palette, mode = %d", m.mode)
	}
	if m.cur().showDots == before {
		t.Error("the command did not run")
	}
}

// A command that opens a window from the palette lands in that window, not
// back in the browse view.
func TestPaletteCanOpenAnotherWindow(t *testing.T) {
	m, _ := paletteModel(t)
	m = typeQuery(t, m, "new file")
	m, _ = send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeCreate {
		t.Errorf("mode = %d, want the new-file prompt", m.mode)
	}
}

// Unavailable commands stay listed — the palette answers "can fe do this?" —
// but say why rather than doing nothing when you pick one.
func TestPaletteListsUnavailableCommandsAndExplainsThem(t *testing.T) {
	m, _ := paletteModel(t)
	m = typeQuery(t, m, "paste")
	if len(m.paletteRows) == 0 {
		t.Fatal("paste is not listed when the clipboard is empty")
	}
	if c, _ := m.paletteSelected(); c.available(m) {
		t.Fatal("paste should be unavailable here")
	}

	m, _ = send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeBrowse {
		t.Error("picking an unavailable command should still close the palette")
	}
	if !strings.Contains(m.status, "nothing to act on") {
		t.Errorf("status = %q, want an explanation", m.status)
	}
}

func TestPaletteEscClosesIt(t *testing.T) {
	m, _ := paletteModel(t)
	m = typeQuery(t, m, "zip")
	m, _ = send(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != modeBrowse {
		t.Errorf("esc should close the palette, mode = %d", m.mode)
	}
	if m.ti.Value() != "" {
		t.Errorf("the query outlived the palette: %q", m.ti.Value())
	}
}

// The box must not change height as the list narrows, or it jumps about under
// the cursor while you type.
func TestPaletteKeepsOneHeightWhileTyping(t *testing.T) {
	m, _ := paletteModel(t)
	want := lipgloss.Height(m.paletteBox())
	for _, q := range []string{"z", "zi", "zip", "zzzz"} {
		mm, _ := paletteModel(t)
		mm = typeQuery(t, mm, q)
		if h := lipgloss.Height(mm.paletteBox()); h != want {
			t.Errorf("%q: box is %d lines, want %d", q, h, want)
		}
	}
}

func TestPaletteBoxIsRectangularAndFits(t *testing.T) {
	for _, s := range []struct{ w, h int }{{100, 24}, {60, 20}, {40, 12}, {32, 9}} {
		m, _ := selModel(t)
		m.width, m.height = s.w, s.h
		m.layout()
		m, _ = send(t, m, keyRune(':'))

		box := m.paletteBox()
		assertRectangular(t, box)
		if w := lipgloss.Width(box); w > m.width {
			t.Errorf("%dx%d: palette is %d wide", s.w, s.h, w)
		}
		view := m.View()
		if h := lipgloss.Height(view); h != m.height {
			t.Errorf("%dx%d: view is %d lines", s.w, s.h, h)
		}
		for i, l := range strings.Split(view, "\n") {
			if w := ansi.StringWidth(strip(l)); w > m.width {
				t.Errorf("%dx%d: line %d is %d cells", s.w, s.h, i, w)
			}
		}
	}
}

// The footer drops the browse hints while the palette is up — they don't apply.
func TestFooterYieldsToThePalette(t *testing.T) {
	m, _ := paletteModel(t)
	if got := strip(m.footer()); got != "" {
		t.Errorf("footer = %q, want empty while the palette is open", got)
	}
}
