package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	popupPad     = 6  // terminal columns left clear either side of a popup
	popupMinW    = 28 // narrower than this and nothing is readable anyway
	popupMaxRows = 14 // most list rows a popup shows before it starts scrolling
)

// popupInner clamps a popup's preferred content width to what the terminal can
// actually show.
func (m model) popupInner(want int) int {
	if lim := m.width - popupPad; want > lim {
		want = lim
	}
	if want < popupMinW {
		want = popupMinW
	}
	return want
}

// popupBox frames a floating window: a title line, the body, and a hint line,
// every line padded to inner columns so the right border stays straight.
func popupBox(title, hint string, body []string, inner int) string {
	lines := make([]string, 0, len(body)+2)
	lines = append(lines, titleStyle.Render(padLine(" "+title, inner)))
	for _, b := range body {
		lines = append(lines, padLine(b, inner))
	}
	if hint != "" {
		lines = append(lines, hintStyle.Render(padLine(" "+hint, inner)))
	}
	return popupBorder.Render(strings.Join(lines, "\n"))
}

// padLine pads or trims an already-styled line to exactly w display columns.
// It measures display width rather than counting runes, so escape sequences in
// the line don't throw the padding off.
func padLine(s string, w int) string {
	switch d := w - ansi.StringWidth(s); {
	case d > 0:
		return s + strings.Repeat(" ", d)
	case d < 0:
		return ansi.Truncate(s, w, "…") + reset
	}
	return s
}

// floatOver centers a popup on the two-pane view, which stays visible around it.
func (m model) floatOver(box string) string {
	x := (m.width - lipgloss.Width(box)) / 2
	y := (m.height - lipgloss.Height(box)) / 2
	if y < 0 {
		y = 0
	}
	return overlayBox(m.dualView(), box, x, y, m.width)
}
