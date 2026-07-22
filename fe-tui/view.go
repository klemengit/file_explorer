package main

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
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

func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= w {
		return s
	}
	rs := []rune(s)
	if w == 1 {
		return "…"
	}
	return string(rs[:w-1]) + "…"
}

func padRight(s string, w int) string {
	n := utf8.RuneCountInString(s)
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
	return m.dualView()
}

// drivesView floats the drives window in the middle of the two panes, which
// stay visible (dimmed only by being behind it) underneath.
func (m model) drivesView() string {
	box := m.drivesBox()
	x := (m.width - lipgloss.Width(box)) / 2
	y := (m.height - lipgloss.Height(box)) / 2
	if y < 0 {
		y = 0
	}
	return overlayBox(m.dualView(), box, x, y, m.width)
}

const (
	drivesWidth  = 62 // preferred inner width of the drives popup
	driveSizeW   = 7
	driveFSW     = 6
	driveFixedW  = 3 + driveSizeW + driveFSW + 4 // gutter + size + fstype + gaps
	drivesMinRow = 6
)

// drivesBox renders the popup itself: a title, one line per drive, a detail
// line for the highlighted drive, and the key hints.
func (m model) drivesBox() string {
	inner := drivesWidth
	if lim := m.width - 6; inner > lim {
		inner = lim
	}
	if inner < 28 {
		inner = 28
	}

	title := "drives"
	if n := len(m.drives); n > 0 {
		title = fmt.Sprintf("drives (%d)", n)
	}
	lines := []string{titleStyle.Render(padRight(" "+title, inner))}
	if len(m.drives) == 0 {
		lines = append(lines, hintStyle.Render(padRight("  no external drives found", inner)))
	} else {
		top, n := m.driveWindow()
		for i := top; i < top+n; i++ {
			lines = append(lines, m.driveRow(i, inner))
		}
	}
	lines = append(lines, m.driveStatusLine(inner))
	hint := " enter open · u unmount · e eject · r refresh · esc close"
	lines = append(lines, hintStyle.Render(padRight(truncate(hint, inner), inner)))

	return drivesBorder.Render(strings.Join(lines, "\n"))
}

// driveWindow returns the first visible drive index and how many fit, scrolling
// to keep the cursor in view when there are more drives than room.
func (m model) driveWindow() (top, n int) {
	// Border, title, detail and hint lines all come off the terminal height.
	avail := m.height - 7
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

func (m model) footer() string {
	switch m.mode {
	case modeFilter, modeRename, modeOpenWith, modeArchive:
		return promptStyle.Render(m.ti.View())
	case modeConfirm:
		return warnStyle.Render(m.confirmMsg + "  (y/n)")
	}
	if m.status != "" {
		switch m.statusLv {
		case lvlWarn:
			return warnStyle.Render(m.status)
		case lvlErr:
			return errStyle.Render(m.status)
		default:
			return statusStyle.Render(m.status)
		}
	}
	hint := "hjkl move · tab switch pane · V/space select · F5/F6 copy/move · / filter · f find · y/x/p yank/cut/paste · r rename · d delete · ? help · q quit"
	return hintStyle.Render(truncate(hint, m.width))
}

func (m model) pickerView() string {
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

	hint := "  type to filter · ↑↓ move · enter select · esc cancel"
	if m.pickerKind == pickBookmarks {
		hint += " · ctrl-d delete"
	}
	b.WriteString(hintStyle.Render(truncate(hint, m.width)))
	return b.String()
}

func (m model) helpView() string {
	type kb struct{ key, desc string }
	binds := []kb{
		{"tab", "switch active pane"},
		{"F5 / F6", "copy / move to other pane"},
		{"h / ←", "parent directory"},
		{"j / ↓", "down"},
		{"k / ↑", "up"},
		{"l / → / enter", "enter directory / open file"},
		{"gg", "go to top"},
		{"G", "go to bottom"},
		{"ctrl-d / ctrl-u", "half page down / up"},
		{"V", "visual select (j/k extend, V keeps)"},
		{"space", "select / deselect, move down"},
		{"esc", "leave visual / clear selection"},
		{"O", "open with… (app menu)"},
		{"e", "edit in nvim"},
		{"E", "open current dir in file manager"},
		{"y", "yank (copy)"},
		{"x", "cut"},
		{"p", "paste here"},
		{"c", "copy path / name to clipboard"},
		{"d", "delete (confirm)"},
		{"r", "rename"},
		{"z", "zip / unzip"},
		{"s / /", "filter (type; esc exits)"},
		{"t", "sort name / newest"},
		{".", "show / hide dotfiles"},
		{"D", "show / hide directories"},
		{"f", "deep find"},
		{"n", "cycle sort: newest → oldest → name"},
		{"m", "bookmark dir"},
		{"b", "jump to bookmark"},
		{"M", "external drives (u unmount, e eject)"},
		{"?", "toggle this help"},
		{"q / ctrl-c", "quit"},
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render("  fe — keybindings"))
	b.WriteString("\n\n")
	for _, k := range binds {
		b.WriteString("  ")
		b.WriteString(helpKey.Render(padRight(k.key, 18)))
		b.WriteString(helpDesc.Render(k.desc))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(hintStyle.Render("  press ? or esc to close"))
	return b.String()
}
