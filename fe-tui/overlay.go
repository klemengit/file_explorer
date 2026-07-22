package main

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// overlayBox draws fg on top of bg with its top-left corner at (x, y), the way
// a floating window sits over the panes. Both sides keep their styling: the
// background is cut by display width with its escape sequences intact, and each
// spliced line is fenced with resets so neither side's colors bleed into the
// other.
//
// Rows of fg past the bottom of bg are dropped, and maxW (the terminal width;
// 0 for no limit) clips every composed line — an overlay must never make the
// frame taller or wider than the screen, or the terminal wraps it and the whole
// display shears.
func overlayBox(bg, fg string, x, y, maxW int) string {
	if fg == "" {
		return bg
	}
	if x < 0 {
		x = 0
	}
	bgLines := strings.Split(bg, "\n")
	fgLines := strings.Split(fg, "\n")

	for i, fl := range fgLines {
		row := y + i
		if row < 0 || row >= len(bgLines) {
			continue
		}
		base := bgLines[row]
		w := ansi.StringWidth(fl)

		left := ansi.Truncate(base, x, "")
		if pad := x - ansi.StringWidth(left); pad > 0 {
			left += strings.Repeat(" ", pad) // background line ends before the box
		}
		right := ansi.TruncateLeft(base, x+w, "")

		line := left + reset + fl + reset + right
		if maxW > 0 && ansi.StringWidth(line) > maxW {
			line = ansi.Truncate(line, maxW, "") + reset
		}
		bgLines[row] = line
	}
	return strings.Join(bgLines, "\n")
}

const reset = "\x1b[0m"
