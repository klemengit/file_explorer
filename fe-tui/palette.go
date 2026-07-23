package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// The command palette (:) is the whole command registry made searchable. It
// exists for discovery rather than speed — every command it lists is already
// one keypress away, and the palette shows you which one, so using it teaches
// you not to need it. It is also where a command that doesn't deserve a letter
// of its own can still live.

const (
	paletteW    = 58 // preferred inner width
	paletteRowN = 12 // most rows shown before the list scrolls
)

// openPalette enters palette mode with the whole registry listed.
func (m model) openPalette() (tea.Model, tea.Cmd) {
	m.mode = modePalette
	m.paletteCursor = 0
	m.paletteTop = 0
	m.ti.Prompt = "› "
	m.ti.SetValue("")
	m.ti.Placeholder = "type a command…"
	m.paletteApplyFilter()
	return m, m.ti.Focus()
}

// paletteItems is what the palette lists: every command that isn't a pure
// motion, in registry order.
func paletteItems() []int {
	out := make([]int, 0, len(commandSet))
	for i, c := range commandSet {
		if !c.hidden {
			out = append(out, i)
		}
	}
	return out
}

// paletteHaystack is the text a query is matched against: what the row shows,
// plus the command's own keys and its unshown synonyms, so "mkdir" finds "new
// file" and "^d" finds delete.
func paletteHaystack(c command) string {
	return c.desc + " " + c.alt + " " + c.prettyKeys()
}

func (m *model) paletteApplyFilter() {
	q := m.ti.Value()
	m.paletteRows = m.paletteRows[:0]
	for _, i := range paletteItems() {
		if q == "" || fuzzyMatch(q, paletteHaystack(commandSet[i])) {
			m.paletteRows = append(m.paletteRows, i)
		}
	}
	if m.paletteCursor >= len(m.paletteRows) {
		m.paletteCursor = len(m.paletteRows) - 1
	}
	if m.paletteCursor < 0 {
		m.paletteCursor = 0
	}
	m.paletteClampScroll()
}

// paletteHeight is how many rows fit at once. It is sized from the unfiltered
// list rather than the filtered one, so the box keeps one height while you
// type instead of shuddering on every keystroke.
func (m model) paletteHeight() int {
	h := len(paletteItems())
	if h > paletteRowN {
		h = paletteRowN
	}
	if lim := m.height - 6; h > lim { // borders, title, input, hint, slack
		h = lim
	}
	if h < 1 {
		h = 1
	}
	return h
}

func (m *model) paletteClampScroll() {
	h := m.paletteHeight()
	if m.paletteCursor < m.paletteTop {
		m.paletteTop = m.paletteCursor
	}
	if m.paletteCursor >= m.paletteTop+h {
		m.paletteTop = m.paletteCursor - h + 1
	}
	if m.paletteTop > len(m.paletteRows)-h {
		m.paletteTop = len(m.paletteRows) - h
	}
	if m.paletteTop < 0 {
		m.paletteTop = 0
	}
}

// paletteSelected is the command under the cursor.
func (m model) paletteSelected() (command, bool) {
	if m.paletteCursor < 0 || m.paletteCursor >= len(m.paletteRows) {
		return command{}, false
	}
	return commandSet[m.paletteRows[m.paletteCursor]], true
}

func (m model) updatePalette(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeBrowse
		m.ti.SetValue("")
		return m, nil
	case "enter":
		c, ok := m.paletteSelected()
		if !ok {
			return m, nil
		}
		m.mode = modeBrowse
		m.ti.SetValue("")
		m.status = ""
		if !c.available(m) {
			// Listing it anyway is the point — the palette answers "can fe do
			// this?" — but running it here would do nothing silently.
			m.setStatus(lvlWarn, "%s: nothing to act on here", c.desc)
			return m, nil
		}
		return c.run(m)
	case "up", "ctrl+k", "ctrl+p":
		m.paletteCursor--
		if m.paletteCursor < 0 {
			m.paletteCursor = 0
		}
		m.paletteClampScroll()
		return m, nil
	case "down", "ctrl+j", "ctrl+n":
		m.paletteCursor++
		if m.paletteCursor >= len(m.paletteRows) {
			m.paletteCursor = len(m.paletteRows) - 1
		}
		m.paletteClampScroll()
		return m, nil
	}
	var cmd tea.Cmd
	m.ti, cmd = m.ti.Update(msg)
	m.paletteApplyFilter()
	return m, cmd
}

func (m model) paletteView() string { return m.floatOver(m.paletteBox()) }

// paletteKeyW is the width of the key column: as wide as the widest set of
// keys in the palette, so every row's keys line up on the right.
func paletteKeyW() int {
	w := 3
	for _, i := range paletteItems() {
		if n := ansi.StringWidth(commandSet[i].prettyKeys()); n > w {
			w = n
		}
	}
	return w
}

// paletteBox renders the window: the query, the matching commands with the key
// that runs each one, and the hints.
func (m model) paletteBox() string {
	inner := m.popupInner(paletteW)
	keyW := paletteKeyW()

	// The input is part of the box, so it scrolls within the box's width:
	// leading space + the two-cell prompt + Width + a cursor cell.
	m.ti.Width = inner - 4
	body := []string{promptStyle.Render(" " + m.ti.View())}

	h := m.paletteHeight()
	rows := make([]string, 0, h)
	for i := m.paletteTop; i < m.paletteTop+h && i < len(m.paletteRows); i++ {
		rows = append(rows, m.paletteRow(i, inner, keyW))
	}
	if len(m.paletteRows) == 0 {
		rows = append(rows, hintStyle.Render(padLine("   no command matches", inner)))
	}
	// Blank out the unused rows rather than dropping them, so the box keeps one
	// height while you type instead of jumping about under the cursor.
	for len(rows) < h {
		rows = append(rows, padLine("", inner))
	}
	body = append(body, rows...)

	hint := "↑↓ move · enter run · esc close"
	if len(m.paletteRows) > h {
		hint = fmt.Sprintf("%d matches · %s", len(m.paletteRows), hint)
	}
	return popupBox("commands", hint, body, inner)
}

// paletteRow draws one command: a gutter, what it does, and the key that does
// it. A command that can't run here is dimmed rather than dropped, so the list
// stays the same shape whatever the cursor is sitting on.
func (m model) paletteRow(i, inner, keyW int) string {
	c := commandSet[m.paletteRows[i]]
	descW := inner - 3 - 2 - keyW
	if descW < 1 {
		descW = 1
	}

	gutter := "   "
	if i == m.paletteCursor {
		gutter = " ▶ "
	}
	desc := padRight(truncate(c.desc, descW), descW)
	keys := padLeft(truncate(c.prettyKeys(), keyW), keyW)

	switch {
	case i == m.paletteCursor:
		return cursorStyle.Render(padLine(gutter+desc+"  "+keys, inner))
	case !c.available(m):
		return hintStyle.Render(padLine(gutter+desc+"  "+keys, inner))
	}
	return fileStyle.Render(gutter+desc) + metaStyle.Render("  "+keys)
}

// padLeft right-aligns s in w terminal cells.
func padLeft(s string, w int) string {
	if n := ansi.StringWidth(s); n < w {
		return strings.Repeat(" ", w-n) + s
	}
	return s
}
