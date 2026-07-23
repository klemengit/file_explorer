package main

import (
	"github.com/charmbracelet/x/ansi"
)

// The prompts — filter, rename, new, zip as, open with — and the y/n confirm
// are floating windows over the panes rather than a line in the footer, the
// way a command line does in Neovim or an input does in yazi. They sit in the
// same frame as the drives, help and picker windows, so every modal thing in
// fe looks the same.

const (
	// promptMinW keeps a prompt window wide enough to type a filename into
	// even when its title and hint are short.
	promptMinW = 48
	// promptMaxW stops a long question stretching the window across the whole
	// terminal. The context line never widens the box at all — a deep path
	// would swallow the screen — it is cut to fit instead.
	promptMaxW = 68
)

// promptModal reports whether the current mode puts a prompt window over the
// panes. The panes keep updating underneath — a filter shows its matches while
// you type.
func (m model) promptModal() bool {
	switch m.mode {
	case modeFilter, modeRename, modeCreate, modeOpenWith, modeArchive, modeConfirm:
		return true
	}
	return false
}

// promptTitle names the window. It replaces the "rename: " style prefixes the
// input used to carry, which would only repeat the title inside the box.
func (m model) promptTitle() string {
	switch m.mode {
	case modeFilter:
		return "filter"
	case modeRename:
		return "rename"
	case modeCreate:
		return "new"
	case modeArchive:
		return "zip as"
	case modeOpenWith:
		return "open with"
	case modeConfirm:
		return "confirm"
	}
	return ""
}

// promptHint is the key line along the bottom of the window.
func (m model) promptHint() string {
	switch m.mode {
	case modeConfirm:
		return "y / enter confirm · n / esc cancel"
	case modeFilter:
		// enter doesn't just accept the filter — it opens whatever the filter
		// left under the cursor.
		return "↑↓ move · enter open · esc clear"
	}
	return "enter confirm · esc cancel"
}

// promptContext is the line above the input saying what is being acted on —
// which name is being replaced, which directory a new file lands in. The
// filter needs none (the panes behind it are the answer) and the confirm's
// question is its body.
func (m model) promptContext() string {
	switch m.mode {
	case modeRename, modeCreate, modeArchive, modeOpenWith:
		return m.promptSubject
	}
	return ""
}

// promptNaturalW is the width the window would like: enough for its title,
// hint and question, never so narrow that typing is cramped, and never wide
// enough to take over the screen.
func (m model) promptNaturalW() int {
	w := promptMinW
	for _, s := range []string{m.promptTitle(), m.promptHint(), m.confirmMsg} {
		if n := ansi.StringWidth(s) + 3; n > w {
			w = n
		}
	}
	if w > promptMaxW {
		w = promptMaxW
	}
	return w
}

// truncateLeft cuts s to w cells from the left, marking the cut with a leading
// ellipsis. Paths are the reason it exists: "…/file_exp/fe-tui" says far more
// about where you are than the first w characters of "/home/…" would.
func truncateLeft(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if ansi.StringWidth(s) <= w {
		return s
	}
	return "…" + ansi.TruncateLeft(s, ansi.StringWidth(s)-w+1, "")
}

// promptView floats the prompt window over the two panes.
func (m model) promptView() string { return m.floatOver(m.promptBox()) }

// promptBox renders the window itself: title, what it is acting on, the input
// (or the question, when confirming), and the key hints.
func (m model) promptBox() string {
	inner := m.popupInner(m.promptNaturalW())

	var body []string
	if c := m.promptContext(); c != "" {
		body = append(body, hintStyle.Render(padLine(" "+truncateLeft(c, inner-1), inner)))
	}
	if m.mode == modeConfirm {
		body = append(body, warnStyle.Render(padLine(" "+m.confirmMsg, inner)))
	} else {
		// The input lives inside the box, so it scrolls within the box's
		// width: leading space + the two-cell prompt + Width + a cursor cell.
		m.ti.Width = inner - 4
		body = append(body, promptStyle.Render(" "+m.ti.View()))
	}
	return popupBox(m.promptTitle(), m.promptHint(), body, inner)
}
