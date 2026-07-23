package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// A chord is a key that waits for a second key instead of acting on its own:
// press g, then d, and the pane jumps to Downloads. Only goto is a chord today,
// but nothing here knows that — a new prefix is an entry in chordSet rather
// than another special case in the browse switch, and everything that explains
// a chord to you (the footer hint, the which-key window) is built from the same
// entries.

// chordEntry is one continuation of a chord: the key that may follow the
// prefix, and what pressing it does.
type chordEntry struct{ key, label string }

// chord is a prefix key together with the keys that may follow it.
type chord struct {
	key   string // the prefix
	title string // names the chord in the hint line and the which-key window

	// entries lists the continuations. It takes the model because a chord's
	// keys can depend on the machine: goto only offers the directories that
	// actually exist.
	entries func(model) []chordEntry

	// run handles the key that followed the prefix. It always consumes that
	// key, so `gd` is "go to Downloads" and never "delete".
	run func(model, string) (tea.Model, tea.Cmd)
}

var chordSet = []chord{{
	key:     "g",
	title:   "goto",
	entries: gotoEntries,
	run:     func(m model, key string) (tea.Model, tea.Cmd) { return m.gotoChord(key) },
}}

// chordFor looks up the chord a key starts.
func chordFor(key string) (chord, bool) {
	for _, c := range chordSet {
		if c.key == key {
			return c, true
		}
	}
	return chord{}, false
}

// pendingChord is the chord waiting for its second key, if one is.
func (m model) pendingChord() (chord, bool) {
	if m.pending == "" {
		return chord{}, false
	}
	return chordFor(m.pending)
}

// whichKeyDelay is how long a pending chord waits before its window appears.
// Typed at speed, `gd` resolves well inside it and nothing ever pops up;
// hesitate, and the window arrives to say what the keys are. Which is the
// bargain a which-key plugin makes in vim, and the reason the delay exists at
// all — a window that opened instantly would flash on every chord you already
// know.
const whichKeyDelay = 400 * time.Millisecond

// whichKeyMsg fires when a pending chord has gone unanswered long enough to be
// worth explaining. gen identifies the chord that asked for it: arming or
// disarming a chord bumps the model's generation, so a tick queued by a chord
// that has since been answered arrives stale and is dropped instead of popping
// a window over whatever you are doing now.
type whichKeyMsg struct{ gen int }

func whichKeyAfter(gen int) tea.Cmd {
	return tea.Tick(whichKeyDelay, func(time.Time) tea.Msg { return whichKeyMsg{gen} })
}

// startChord arms a chord and schedules its window.
func (m *model) startChord(key string) tea.Cmd {
	m.pending = key
	m.whichKey = false
	m.chordGen++
	return whichKeyAfter(m.chordGen)
}

// endChord disarms the pending chord, invalidating any window tick still in
// flight for it.
func (m *model) endChord() {
	m.pending = ""
	m.whichKey = false
	m.chordGen++
}

// chordHint is the one-line reminder shown in the footer while a chord is
// pending. Where the terminal is wide enough every continuation is named;
// where it isn't the bare keys are listed instead, which at least says what is
// bound — the which-key window and ? have the full list either way.
func chordHint(title string, entries []chordEntry, width int) string {
	named := make([]string, 0, len(entries)+1)
	keys := make([]string, 0, len(entries))
	for _, e := range entries {
		named = append(named, e.key+" "+e.label)
		keys = append(keys, e.key)
	}
	full := title + ": " + strings.Join(append(named, "esc cancel"), " · ")
	if width <= 0 || ansi.StringWidth(full) <= width {
		return full
	}
	return title + ": " + strings.Join(keys, " ") + " · esc cancel"
}

const (
	whichKeyGap  = 2 // columns between which-key columns
	whichKeyRows = 8 // rows to prefer before spilling into another column
)

// whichKeyView floats the which-key window over the two panes, which stay
// visible around it — the chord acts on what you can see.
func (m model) whichKeyView() string { return m.floatOver(m.whichKeyBox()) }

// whichKeyLayout works out how the pending chord's continuations are arranged
// for the current terminal: how many columns they spill into, how wide the key
// and label halves of a cell are, and how many rows fit on screen.
func (m model) whichKeyLayout(entries []chordEntry) (cols, rows, keyW, labelW, inner int) {
	keyW, want := 1, 1
	for _, e := range entries {
		if n := ansi.StringWidth(e.key); n > keyW {
			keyW = n
		}
		if n := ansi.StringWidth(e.label); n > want {
			want = n
		}
	}
	cell := keyW + 2 + want

	// How many columns of that width the terminal has room for.
	fit := 1
	for c := 2; 2+c*cell+(c-1)*whichKeyGap <= m.width-popupPad; c++ {
		fit = c
	}

	// Prefer a short list over a wide one: spill into another column only once
	// the rows run past what is comfortable, or past the screen.
	maxRows := m.height - 5 // borders, title, hint, and a line of slack
	if maxRows > whichKeyRows {
		maxRows = whichKeyRows
	}
	if maxRows < 1 {
		maxRows = 1
	}
	cols = (len(entries) + maxRows - 1) / maxRows
	if cols < 1 {
		cols = 1
	}
	if cols > fit {
		cols = fit
	}
	rows = (len(entries) + cols - 1) / cols
	if lim := m.height - 5; lim > 0 && rows > lim {
		rows = lim // more chords than screen; the hint says how many are left
	}
	if rows < 1 {
		rows = 1
	}

	inner = m.popupInner(2 + cols*cell + (cols-1)*whichKeyGap)
	// Share out whatever width we actually got between the columns.
	labelW = (inner-2-(cols-1)*whichKeyGap)/cols - keyW - 2
	if labelW < 1 {
		labelW = 1
	}
	return cols, rows, keyW, labelW, inner
}

// whichKeyBox renders the window: the chord's continuations in as many columns
// as fit, filled column-major so the list reads down and then across.
func (m model) whichKeyBox() string {
	c, ok := m.pendingChord()
	if !ok {
		return ""
	}
	entries := c.entries(m)
	cols, rows, keyW, labelW, inner := m.whichKeyLayout(entries)

	body := make([]string, 0, rows)
	for r := 0; r < rows; r++ {
		var line strings.Builder
		line.WriteString(" ")
		for col := 0; col < cols; col++ {
			if col > 0 {
				line.WriteString(strings.Repeat(" ", whichKeyGap))
			}
			i := col*rows + r // column-major
			if i >= len(entries) {
				continue // nothing past the end needs padding; padLine squares it
			}
			line.WriteString(helpKey.Render(padRight(entries[i].key, keyW)))
			line.WriteString("  ")
			line.WriteString(helpDesc.Render(padRight(truncate(entries[i].label, labelW), labelW)))
		}
		body = append(body, line.String())
	}

	hint := "esc cancel"
	if hidden := len(entries) - cols*rows; hidden > 0 {
		hint = fmt.Sprintf("%d more (? lists them) · %s", hidden, hint)
	}
	return popupBox(c.key+" — "+c.title, hint, body, inner)
}
