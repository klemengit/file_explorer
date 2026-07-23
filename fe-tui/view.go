package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// abbrevHome shortens the home-directory prefix of an absolute path to "~"
// (display only; relative paths are returned unchanged).
func abbrevHome(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == home {
		return "~"
	}
	if strings.HasPrefix(p, home+"/") {
		return "~" + p[len(home):]
	}
	return p
}

// truncate cuts s down to w terminal cells, ending it in an ellipsis when
// something had to go.
//
// Cells, not runes: an emoji or a CJK character is one rune but takes two
// columns, so a filename like "⚡ Dashboard.md" measured by rune count comes
// out a column narrower than it draws. A row built to that measurement
// overflows its pane by a column, lipgloss wraps it rather than clipping it,
// and the box grows a line taller than its neighbour — which is how one
// emoji-heavy directory knocks the whole two-pane layout out of alignment.
func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if ansi.StringWidth(s) <= w {
		return s
	}
	return ansi.Truncate(s, w, "…")
}

// padRight pads s with spaces out to w terminal cells (see truncate on why
// cells rather than runes).
func padRight(s string, w int) string {
	n := ansi.StringWidth(s)
	if n >= w {
		return s
	}
	return s + strings.Repeat(" ", w-n)
}

func fgColorFor(r row) lipgloss.Color {
	switch {
	case r.isParent:
		return lipgloss.Color(colComment)
	case r.isDir:
		return lipgloss.Color(colBlue)
	case r.isLink:
		return lipgloss.Color(colCyan)
	default:
		return lipgloss.Color(colFg)
	}
}

func styleFor(r row) lipgloss.Style {
	switch {
	case r.isParent:
		return parentStyle
	case r.isDir:
		return dirStyle
	case r.isLink:
		return linkStyle
	default:
		return fileStyle
	}
}

func (m model) View() string {
	if m.height == 0 {
		return "" // await first WindowSizeMsg
	}
	if m.showHelp {
		return m.helpView()
	}
	if m.mode == modePicker {
		return m.pickerView()
	}
	if m.mode == modeDrives {
		return m.drivesView()
	}
	if m.mode == modePalette {
		return m.paletteView()
	}
	if m.promptModal() {
		return m.promptView()
	}
	if m.whichKey {
		return m.whichKeyView()
	}
	return m.dualView()
}

// drivesView floats the drives window in the middle of the two panes, which
// stay visible around it.
func (m model) drivesView() string { return m.floatOver(m.drivesBox()) }

const (
	drivesWidth  = 62 // preferred inner width of the drives popup
	driveSizeW   = 7
	driveFSW     = 6
	driveFixedW  = 3 + driveSizeW + driveFSW + 4 // gutter + size + fstype + gaps
	drivesMinRow = 6

	// Layout of the "what is holding this drive" lines shown after a busy
	// unmount. The list is capped so a runaway process count can't push the
	// drives themselves off the popup.
	driveHolderMax    = 6
	driveHolderPIDW   = 7
	driveHolderNameW  = 14
	driveHolderFixedW = 2 + driveHolderPIDW + 2 + driveHolderNameW + 2
)

// drivesBox renders the popup itself: a title, one line per drive, a detail
// line for the highlighted drive, and the key hints.
func (m model) drivesBox() string {
	inner := m.popupInner(drivesWidth)

	title := "drives"
	if n := len(m.drives); n > 0 {
		title = fmt.Sprintf("drives (%d)", n)
	}
	var body []string
	if len(m.drives) == 0 {
		body = append(body, hintStyle.Render("  no external drives found"))
	} else {
		top, n := m.driveWindow()
		for i := top; i < top+n; i++ {
			body = append(body, m.driveRow(i, inner))
		}
	}
	body = append(body, m.driveStatusLine(inner))
	body = append(body, m.driveHolderLines(inner)...)

	hint := "enter open · u unmount · e eject · r refresh · esc close"
	if m.driveStuck != nil {
		hint = "F force " + m.driveStuckOp + " (data loss risk) · r recheck · esc close"
	}
	return popupBox(title, hint, body, inner)
}

// driveWindow returns the first visible drive index and how many fit, scrolling
// to keep the cursor in view when there are more drives than room.
func (m model) driveWindow() (top, n int) {
	// Border, title, detail and hint lines all come off the terminal height,
	// as do the "what's holding it" lines when a busy unmount put them there.
	avail := m.height - 7 - m.driveHolderRows()
	if avail < drivesMinRow {
		avail = drivesMinRow
	}
	if len(m.drives) <= avail {
		return 0, len(m.drives)
	}
	top = m.driveCursor - avail/2
	if top > len(m.drives)-avail {
		top = len(m.drives) - avail
	}
	if top < 0 {
		top = 0
	}
	return top, avail
}

// driveRow draws one drive as: gutter, label, size, filesystem, mountpoint.
// Every segment is padded to a fixed width before styling, so the popup's right
// border stays straight.
func (m model) driveRow(i, inner int) string {
	d := m.drives[i]

	rest := inner - driveFixedW
	nameW := rest * 45 / 100
	if nameW < 6 {
		nameW = 6
	}
	locW := rest - nameW

	gutter := "   "
	if i == m.driveCursor {
		gutter = " ▶ "
	}
	size := "-"
	if d.size >= 0 {
		size = humanSize(d.size)
	}
	loc := "not mounted"
	if d.mounted() {
		loc = abbrevHome(d.mount)
	}

	name := padRight(truncate(d.label, nameW), nameW)
	meta := fmt.Sprintf(" %*s %s %s ",
		driveSizeW, truncate(size, driveSizeW),
		padRight(truncate(d.fstype, driveFSW), driveFSW),
		padRight(truncate(loc, locW), locW))

	if i == m.driveCursor {
		return cursorStyle.Foreground(lipgloss.Color(colFg)).Render(gutter + name + meta)
	}
	nameStyle := fileStyle
	if d.mounted() {
		nameStyle = dirStyle // mounted drives read like directories you can enter
	}
	return hintStyle.Render(gutter) + nameStyle.Render(name) + metaStyle.Render(meta)
}

// driveHolderRows is how many lines driveHolderLines will produce, including
// the "… and N more" line. driveWindow needs this before the rows are built so
// the drive list can give up the space.
func (m model) driveHolderRows() int {
	if m.driveStuck == nil || len(m.driveHolders) == 0 {
		return 0
	}
	if n := len(m.driveHolders); n > driveHolderMax {
		return driveHolderMax + 1 // capped rows plus the "… and N more" line
	}
	return len(m.driveHolders)
}

// driveHolderLines lists the processes keeping a stuck drive busy, one per
// line under the status line: PID, command, and the path it is holding. That
// last column is what actually lets you go and close the right thing.
func (m model) driveHolderLines(inner int) []string {
	if m.driveHolderRows() == 0 {
		return nil
	}
	hs := m.driveHolders
	extra := 0
	if len(hs) > driveHolderMax {
		extra, hs = len(hs)-driveHolderMax, hs[:driveHolderMax]
	}

	pathW := inner - driveHolderFixedW
	if pathW < 8 {
		pathW = 8
	}
	out := make([]string, 0, len(hs)+1)
	for _, h := range hs {
		// Show the path relative to the mountpoint: the mountpoint prefix is
		// the same on every row and eats the width that tells them apart.
		rel := strings.TrimPrefix(h.what, m.driveStuck.mount)
		if rel == "" {
			rel = "/"
		}
		line := fmt.Sprintf("  %*d  %s  %s",
			driveHolderPIDW, h.pid,
			padRight(truncate(h.name, driveHolderNameW), driveHolderNameW),
			padRight(truncate(rel, pathW), pathW))
		out = append(out, warnStyle.Render(padRight(truncate(line, inner), inner)))
	}
	if extra > 0 {
		out = append(out, hintStyle.Render(padRight(fmt.Sprintf("  … and %d more", extra), inner)))
	}
	return out
}

// driveStatusLine shows whatever matters most right now: a running action, the
// result of the last one, or the highlighted drive's details.
func (m model) driveStatusLine(inner int) string {
	pad := func(s string) string { return padRight(truncate(" "+s, inner), inner) }
	switch {
	case m.driveBusy != "":
		return warnStyle.Render(pad(m.driveBusy))
	case m.driveNote != "":
		st := statusStyle
		switch m.driveNoteLv {
		case lvlWarn:
			st = warnStyle
		case lvlErr:
			st = errStyle
		}
		return st.Render(pad(m.driveNote))
	}
	d, ok := m.currentDrive()
	if !ok {
		return strings.Repeat(" ", inner)
	}
	detail := d.dev
	if d.fstype != "" {
		detail += " · " + d.fstype
	}
	if d.free >= 0 && d.size >= 0 {
		detail += fmt.Sprintf(" · %s free of %s", humanSize(d.free), humanSize(d.size))
	}
	return metaStyle.Render(pad(detail))
}

// dualView renders both panes in side-by-side bordered boxes (the active pane
// highlighted) with the shared footer full-width beneath.
func (m model) dualView() string {
	left := m.paneBox(&m.panes[0], m.active == 0)
	right := m.paneBox(&m.panes[1], m.active == 1)
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	return body + "\n" + m.footer()
}

// paneBox wraps a pane's content in a border — accent when active, dim when not.
func (m model) paneBox(p *pane, active bool) string {
	style := paneBorderDim
	if active {
		style = paneBorderActive
	}
	return style.Width(p.width).Height(p.height - 1).Render(m.paneContent(p))
}

// paneContent renders a pane's header and visible rows as exactly p.height-1
// lines (header + list), without a footer.
func (m model) paneContent(p *pane) string {
	// header
	header := " " + abbrevHome(p.dir)
	var flags []string
	switch p.sortMode {
	case sortTimeDesc:
		flags = append(flags, "newest")
	case sortTimeAsc:
		flags = append(flags, "oldest")
	}
	if p.showDots {
		flags = append(flags, "dotfiles")
	}
	if !p.showDirs {
		flags = append(flags, "no-dirs")
	}
	if p.visual {
		flags = append(flags, "VISUAL")
	}
	if n := len(p.selectedPaths()); n > 0 {
		flags = append(flags, fmt.Sprintf("%d selected", n))
	}
	if len(flags) > 0 {
		header += "  [" + strings.Join(flags, " ") + "]"
	}

	h := p.listHeight()
	lines := make([]string, 0, h+1)
	lines = append(lines, headerStyle.Render(truncate(header, p.width)))

	end := p.top + h
	if end > len(p.rows) {
		end = len(p.rows)
	}
	for i := p.top; i < end; i++ {
		lines = append(lines, p.renderRow(p.rows[i], i == p.cursor, p.isSelected(i)))
	}
	for len(lines) < h+1 {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

const (
	sizeColW = 7
	dateColW = 16 // "2006-01-02 15:04"
	metaColW = sizeColW + 2 + dateColW
)

// humanSize renders a byte count as a compact human-readable string.
func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%c", float64(n)/float64(div), "KMGTPE"[exp])
}

// rowMeta returns the size + modified-date suffix for a row (empty for the
// parent row; size shown as "-" for directories and symlinks).
func rowMeta(r row) string {
	if r.isParent {
		return ""
	}
	size := "-"
	if !r.isDir && !r.isLink {
		size = humanSize(r.size)
	}
	date := ""
	if !r.modTime.IsZero() {
		date = r.modTime.Format("2006-01-02 15:04")
	}
	return fmt.Sprintf("%*s  %s", sizeColW, size, date)
}

// renderRow draws one row. cursor is the highlighted row; marked means the row
// is part of the multi-selection. The two-cell gutter carries both: the cursor
// arrow in the first cell, the selection mark in the second.
func (p pane) renderRow(r row, cursor, marked bool) string {
	// Show the size/date columns only when the pane is wide enough.
	showMeta := p.width >= metaColW+16
	nameAvail := p.width - 2
	if showMeta {
		nameAvail = p.width - 2 - metaColW - 2
	}
	if nameAvail < 4 {
		nameAvail = 4
	}

	label := truncate(r.label, nameAvail)
	meta := ""
	if showMeta {
		meta = rowMeta(r)
	}
	gutter := " "
	if cursor {
		gutter = "▶"
	}
	if marked {
		gutter += "*"
	} else {
		gutter += " "
	}

	if cursor {
		fg := lipgloss.NewStyle().Foreground(fgColorFor(r)).Background(lipgloss.Color(colSelBg)).Bold(true)
		if marked {
			fg = fg.Foreground(lipgloss.Color(colGreen))
		}
		dim := lipgloss.NewStyle().Foreground(lipgloss.Color(colComment)).Background(lipgloss.Color(colSelBg))
		line := fg.Render(gutter + padRight(label, nameAvail))
		if showMeta {
			line += dim.Render("  " + padRight(meta, metaColW))
		}
		return line
	}

	st := styleFor(r)
	if marked {
		st = markStyle
	}
	line := st.Render(gutter + padRight(label, nameAvail))
	if showMeta {
		line += metaStyle.Render("  " + meta)
	}
	return line
}

// statusLine renders m.status in the colour its level calls for.
func (m model) statusLine() string {
	switch m.statusLv {
	case lvlWarn:
		return warnStyle.Render(m.status)
	case lvlErr:
		return errStyle.Render(m.status)
	default:
		return statusStyle.Render(m.status)
	}
}

func (m model) footer() string {
	// A prompt, palette or which-key window carries its own keys, so the footer
	// drops the browse hints that don't apply while one is open — but keeps a
	// status line, which is often what the window is answering.
	if m.promptModal() || m.mode == modePalette || m.whichKey {
		if m.status == "" {
			return ""
		}
		return m.statusLine()
	}
	// A pending chord shows what the next key can do, so the chords don't have
	// to be memorised. Once its window is up the footer stops repeating it.
	if c, ok := m.pendingChord(); ok && !m.whichKey {
		return promptStyle.Render(truncate(chordHint(c.title, c.entries(m), m.width), m.width))
	}
	if m.status != "" {
		return m.statusLine()
	}
	hint := "hjkl move · tab switch pane · V/space select · F5/F6 copy/move · / filter · f find · y/x/p yank/cut/paste · a new · r rename · d delete · ? help · q quit"
	return hintStyle.Render(truncate(hint, m.width))
}

// pickerView draws the picker either as a floating window or, for deep find —
// whose result list is unbounded and full of long paths — as a full screen.
func (m model) pickerView() string {
	if m.pickerPopup() {
		return m.floatOver(m.pickerBox())
	}
	return m.pickerFullView()
}

// pickerPopup reports whether this picker is small enough to float.
func (m model) pickerPopup() bool { return m.pickerKind != pickFind }

// pickerBox renders a picker as a floating window: title, filter input, the
// visible rows, and the key hints.
func (m model) pickerBox() string {
	inner := m.popupInner(m.pickerNaturalW())

	// The input is part of the box, so it scrolls within the box's width.
	// It renders as leading space + prompt ("/ ") + Width + a cursor cell.
	m.ti.Width = inner - 4
	body := []string{promptStyle.Render(" " + m.ti.View())}

	h := m.pickerHeight()
	end := m.pickerTop + h
	if end > len(m.pickerRows) {
		end = len(m.pickerRows)
	}
	for i := m.pickerTop; i < end; i++ {
		body = append(body, m.pickerRow(i, inner))
	}
	if len(m.pickerRows) == 0 {
		body = append(body, hintStyle.Render("   no matches"))
	}
	// Keep the box a constant height while filtering, so it doesn't jump about
	// under the cursor as matches come and go.
	for len(body) < h+1 {
		body = append(body, "")
	}

	return popupBox(m.pickerTitle, m.pickerHint(), body, inner)
}

// pickerRow draws one row of a floating picker.
func (m model) pickerRow(i, inner int) string {
	item := abbrevHome(m.pickerAll[m.pickerRows[i]])
	avail := inner - 3
	text := truncate(item, avail)
	if i == m.pickerCursor {
		st := lipgloss.NewStyle().
			Foreground(lipgloss.Color(colFg)).
			Background(lipgloss.Color(colSelBg)).
			Bold(true)
		return st.Render(" ▶ " + padRight(text, avail))
	}
	return "   " + fileStyle.Render(text)
}

// pickerNaturalW is the width the picker would like: enough for its longest
// item, its title and its hint, before the terminal has its say.
func (m model) pickerNaturalW() int {
	w := ansi.StringWidth(m.pickerHint()) + 2
	if n := ansi.StringWidth(m.pickerTitle) + 2; n > w {
		w = n
	}
	for _, it := range m.pickerAll {
		if n := ansi.StringWidth(abbrevHome(it)) + 4; n > w {
			w = n
		}
	}
	return w
}

func (m model) pickerHint() string {
	hint := "type to filter · ↑↓ move · enter select · esc cancel"
	if m.pickerKind == pickBookmarks {
		hint += " · ctrl-d delete"
	}
	return hint
}

// pickerFullView is the full-screen picker used for deep find.
func (m model) pickerFullView() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(truncate("  "+m.pickerTitle, m.width)))
	b.WriteString("\n")
	b.WriteString(promptStyle.Render(m.ti.View()))
	b.WriteString("\n")

	h := m.pickerHeight()
	end := m.pickerTop + h
	if end > len(m.pickerRows) {
		end = len(m.pickerRows)
	}
	lines := 0
	for i := m.pickerTop; i < end; i++ {
		item := m.pickerAll[m.pickerRows[i]]
		selected := i == m.pickerCursor
		avail := m.width - 2
		text := truncate(abbrevHome(item), avail)
		if selected {
			st := lipgloss.NewStyle().
				Foreground(lipgloss.Color(colFg)).
				Background(lipgloss.Color(colSelBg)).
				Bold(true)
			b.WriteString(st.Render("▶ " + padRight(text, avail)))
		} else {
			b.WriteString("  " + fileStyle.Render(text))
		}
		b.WriteString("\n")
		lines++
	}
	for ; lines < h; lines++ {
		b.WriteString("\n")
	}

	b.WriteString(hintStyle.Render(truncate("  "+m.pickerHint(), m.width)))
	return b.String()
}

type kb struct{ key, desc string }

// helpBindings is the keybinding list shown by ?. It is the command registry
// rendered, plus the goto chords, which depend on which directories this
// machine has.
func (m model) helpBindings() []kb {
	binds := make([]kb, 0, len(commandSet)+len(m.gotos))
	for _, c := range commandSet {
		binds = append(binds, kb{c.prettyKeys(), c.desc})
	}
	for _, t := range m.gotos {
		binds = append(binds, kb{"g " + t.key, "go to " + t.label})
	}
	return binds
}

// helpKeyW is the width of the key column — as wide as the widest label, which
// depends on what happens to be bound.
func (m model) helpKeyW() int {
	w := 8
	for _, b := range m.helpBindings() {
		if n := ansi.StringWidth(b.key); n > w {
			w = n
		}
	}
	return w
}

// helpMinDesc is the narrowest description worth giving a second column to.
const helpMinDesc = 24

// helpLayout works out how the bindings are arranged for the current terminal:
// two columns when they fit at a readable width, one otherwise, plus how many
// rows exist and how many of them are on screen.
func (m model) helpLayout() (cols, colW, inner, rows, visible int) {
	binds := m.helpBindings()
	helpKeyW := m.helpKeyW()
	descW := 0
	for _, k := range binds {
		if n := ansi.StringWidth(k.desc); n > descW {
			descW = n
		}
	}
	full := helpKeyW + 1 + descW

	cols = 1
	if m.width-popupPad >= 2*(helpKeyW+1+helpMinDesc)+4 {
		cols = 2
	}
	inner = m.popupInner(cols*full + (cols-1)*2 + 2)
	// Share out whatever width we actually got between the columns.
	colW = (inner - 2 - (cols-1)*2) / cols
	if colW > full {
		colW = full
	}

	rows = (len(binds) + cols - 1) / cols
	visible = m.height - 5 // borders, title, hint, and a line of slack
	if visible > rows {
		visible = rows
	}
	if visible < 1 {
		visible = 1
	}
	return cols, colW, inner, rows, visible
}

// helpTopMax is the furthest the help list can be scrolled.
func (m model) helpTopMax() int {
	_, _, _, rows, visible := m.helpLayout()
	if top := rows - visible; top > 0 {
		return top
	}
	return 0
}

func (m model) helpView() string { return m.floatOver(m.helpBox()) }

// helpBox lays the bindings out in one or two key/description columns, filled
// column-major, scrolled to m.helpTop.
func (m model) helpBox() string {
	binds := m.helpBindings()
	helpKeyW := m.helpKeyW()
	cols, colW, inner, rows, visible := m.helpLayout()
	descW := colW - helpKeyW - 1
	if descW < 1 {
		descW = 1
	}

	top := m.helpTop
	if max := rows - visible; top > max {
		top = max
	}
	if top < 0 {
		top = 0
	}

	body := make([]string, 0, visible)
	for r := top; r < top+visible; r++ {
		var line strings.Builder
		line.WriteString(" ")
		for c := 0; c < cols; c++ {
			if c > 0 {
				line.WriteString("  ")
			}
			i := c*rows + r // column-major: the list reads down, then across
			if i >= len(binds) {
				line.WriteString(strings.Repeat(" ", colW))
				continue
			}
			line.WriteString(helpKey.Render(padRight(truncate(binds[i].key, helpKeyW), helpKeyW)))
			line.WriteString(" ")
			line.WriteString(helpDesc.Render(padRight(truncate(binds[i].desc, descW), descW)))
		}
		body = append(body, line.String())
	}

	hint := "press ? or esc to close"
	if rows > visible {
		hint = "j/k scroll · " + hint
	}
	return popupBox("fe — keybindings", hint, body, inner)
}
