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
	return m.browseView()
}

func (m model) browseView() string {
	var b strings.Builder

	// header
	header := " " + abbrevHome(m.dir)
	var flags []string
	if m.sortMode == sortTime {
		flags = append(flags, "modified")
	}
	if m.showDots {
		flags = append(flags, "dotfiles")
	}
	if !m.showDirs {
		flags = append(flags, "no-dirs")
	}
	if len(flags) > 0 {
		header += "  [" + strings.Join(flags, " ") + "]"
	}
	b.WriteString(headerStyle.Render(truncate(header, m.width)))
	b.WriteString("\n")

	// list
	h := m.listHeight()
	end := m.top + h
	if end > len(m.rows) {
		end = len(m.rows)
	}
	lines := 0
	for i := m.top; i < end; i++ {
		b.WriteString(m.renderRow(m.rows[i], i == m.cursor))
		b.WriteString("\n")
		lines++
	}
	for ; lines < h; lines++ {
		b.WriteString("\n")
	}

	b.WriteString(m.footer())
	return b.String()
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

func (m model) renderRow(r row, selected bool) string {
	// Show the size/date columns only when the terminal is wide enough.
	showMeta := m.width >= metaColW+16
	nameAvail := m.width - 2
	if showMeta {
		nameAvail = m.width - 2 - metaColW - 2
	}
	if nameAvail < 4 {
		nameAvail = 4
	}

	label := truncate(r.label, nameAvail)
	meta := ""
	if showMeta {
		meta = rowMeta(r)
	}

	if selected {
		fg := lipgloss.NewStyle().Foreground(fgColorFor(r)).Background(lipgloss.Color(colSelBg)).Bold(true)
		dim := lipgloss.NewStyle().Foreground(lipgloss.Color(colComment)).Background(lipgloss.Color(colSelBg))
		line := fg.Render("▶ " + padRight(label, nameAvail))
		if showMeta {
			line += dim.Render("  " + padRight(meta, metaColW))
		}
		return line
	}

	line := "  " + styleFor(r).Render(padRight(label, nameAvail))
	if showMeta {
		line += metaStyle.Render("  " + meta)
	}
	return line
}

func (m model) footer() string {
	switch m.mode {
	case modeFilter, modeRename, modeOpenWith:
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
	hint := "hjkl move · gg/G top/bottom · / filter · f find · n newest · y/x/p yank/cut/paste · r rename · d delete · ? help · q quit"
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

	b.WriteString(hintStyle.Render(truncate("  type to filter · ↑↓ move · enter select · esc cancel", m.width)))
	return b.String()
}

func (m model) helpView() string {
	type kb struct{ key, desc string }
	binds := []kb{
		{"h / ←", "parent directory"},
		{"j / ↓", "down"},
		{"k / ↑", "up"},
		{"l / → / enter", "enter directory / open file"},
		{"gg", "go to top"},
		{"G", "go to bottom"},
		{"ctrl-d / ctrl-u", "half page down / up"},
		{"O", "open with… (prompt)"},
		{"e", "edit in nvim"},
		{"E", "open current dir in file manager"},
		{"y", "yank (copy)"},
		{"x", "cut"},
		{"p", "paste here"},
		{"d", "delete (confirm)"},
		{"r", "rename"},
		{"z", "zip / unzip"},
		{"s / /", "filter (type; esc exits)"},
		{"t", "sort name / modified"},
		{".", "show / hide dotfiles"},
		{"D", "show / hide directories"},
		{"f", "deep find"},
		{"n", "newest files (recursive)"},
		{"m", "bookmark dir"},
		{"b", "jump to bookmark"},
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
