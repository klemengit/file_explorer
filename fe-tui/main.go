package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type mode int

const (
	modeBrowse mode = iota
	modeFilter
	modeRename
	modeOpenWith
	modeConfirm
	modePicker
)

type confirmKind int

const (
	confirmDelete confirmKind = iota
	confirmPaste
)

type pickerKind int

const (
	pickFind pickerKind = iota
	pickBookmarks
	pickOpenWith
	pickCopy
)

const (
	lvlInfo = iota
	lvlWarn
	lvlErr
)

// row is one displayed line (a directory entry or the synthetic ".." parent).
type row struct {
	label    string
	name     string
	isDir    bool
	isLink   bool
	isParent bool
	size     int64
	modTime  time.Time
}

// editFinishedMsg is returned after an external editor (nvim) exits.
type editFinishedMsg struct{ err error }

type model struct {
	dir     string
	allRows []row
	rows    []row
	cursor  int
	top     int
	width   int
	height  int

	sortMode sortMode
	showDirs bool
	showDots bool

	mode mode
	ti   textinput.Model

	confirmKind confirmKind
	confirmPath string
	confirmMsg  string

	clip     *clipEntry
	status   string
	statusLv int
	showHelp bool
	pendingG bool

	pickerKind   pickerKind
	pickerTitle  string
	pickerAll    []string
	pickerRows   []int
	pickerCursor int
	pickerTop    int

	openWith       []app
	openWithTarget string

	copyItems []copyChoice
}

func newModel(dir string) model {
	ti := textinput.New()
	ti.Prompt = "/ "
	m := model{
		dir:      dir,
		showDirs: true,
		showDots: false,
		sortMode: sortName,
		ti:       ti,
	}
	m.reload()
	return m
}

func (m *model) setStatus(lv int, format string, a ...any) {
	m.status = fmt.Sprintf(format, a...)
	m.statusLv = lv
}

// reload re-reads the current directory and rebuilds the row list.
func (m *model) reload() {
	entries, err := listDir(m.dir, m.sortMode, m.showDirs, m.showDots)
	if err != nil {
		m.setStatus(lvlErr, "Cannot read: %v", err)
		entries = nil
	}
	m.allRows = make([]row, 0, len(entries)+1)
	m.allRows = append(m.allRows, row{label: "..", isParent: true})
	for _, e := range entries {
		label := e.name
		if e.isDir {
			label += "/"
		}
		m.allRows = append(m.allRows, row{
			label:   label,
			name:    e.name,
			isDir:   e.isDir,
			isLink:  e.isLink,
			size:    e.size,
			modTime: e.modTime,
		})
	}
	m.applyView()
}

// applyView recomputes m.rows from m.allRows, applying the live filter query
// when in filter mode, then clamps the cursor and scroll.
func (m *model) applyView() {
	if m.mode == modeFilter && m.ti.Value() != "" {
		q := m.ti.Value()
		m.rows = m.rows[:0]
		var kept []row
		for _, r := range m.allRows {
			if r.isParent {
				continue
			}
			if fuzzyMatch(q, r.name) {
				kept = append(kept, r)
			}
		}
		m.rows = kept
	} else {
		m.rows = m.allRows
	}
	m.clamp()
}

func (m *model) clamp() {
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.clampScroll()
}

func (m *model) clampScroll() {
	h := m.listHeight()
	if m.cursor < m.top {
		m.top = m.cursor
	}
	if m.cursor >= m.top+h {
		m.top = m.cursor - h + 1
	}
	if m.top < 0 {
		m.top = 0
	}
}

func (m *model) listHeight() int {
	h := m.height - 2 // header + footer
	if h < 1 {
		h = 1
	}
	return h
}

func (m *model) cursorTo(name string) {
	for i, r := range m.rows {
		if r.name == name {
			m.cursor = i
			m.clampScroll()
			return
		}
	}
}

func (m *model) enterDir(path string) {
	m.dir = path
	m.cursor = 0
	m.top = 0
	m.reload()
}

func (m *model) goParent() {
	parent := filepath.Dir(m.dir)
	if parent == m.dir {
		return
	}
	child := filepath.Base(m.dir)
	m.enterDir(parent)
	m.cursorTo(child)
}

func (m *model) current() (row, bool) {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return row{}, false
	}
	return m.rows[m.cursor], true
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ti.Width = msg.Width - 4
		m.clampScroll()
		return m, nil

	case editFinishedMsg:
		if msg.err != nil {
			m.setStatus(lvlErr, "editor: %v", msg.err)
		}
		m.reload()
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		switch m.mode {
		case modeBrowse:
			return m.updateBrowse(msg)
		case modeFilter:
			return m.updateFilter(msg)
		case modeRename, modeOpenWith:
			return m.updatePrompt(msg)
		case modeConfirm:
			return m.updateConfirm(msg)
		case modePicker:
			return m.updatePicker(msg)
		}
	}
	return m, nil
}

func (m model) updateBrowse(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if m.showHelp {
		switch key {
		case "?", "esc", "q":
			m.showHelp = false
		}
		return m, nil
	}

	// gg chord: a pending 'g' followed by another 'g' jumps to the top.
	if key != "g" {
		m.pendingG = false
	}
	m.status = "" // any command clears a stale status line

	switch key {
	case "q":
		return m, tea.Quit
	case "j", "down":
		m.move(1)
	case "k", "up":
		m.move(-1)
	case "ctrl+d":
		m.move(m.listHeight() / 2)
	case "ctrl+u":
		m.move(-m.listHeight() / 2)
	case "pgdown":
		m.move(m.listHeight())
	case "pgup":
		m.move(-m.listHeight())
	case "g":
		if m.pendingG {
			m.cursor, m.top = 0, 0
			m.pendingG = false
		} else {
			m.pendingG = true
		}
	case "G":
		m.cursor = len(m.rows) - 1
		m.clampScroll()
	case "h", "left":
		m.goParent()
	case "l", "right", "enter":
		return m.openSelected()
	case "E":
		if err := openDetached("xdg-open", m.dir); err != nil {
			m.setStatus(lvlErr, "xdg-open: %v", err)
		}
	case "O":
		if target, ok := m.selectedTarget(); ok {
			return m.openOpenWith(target)
		}
	case "e":
		if target, ok := m.selectedTarget(); ok {
			c := exec.Command("nvim", target)
			return m, tea.ExecProcess(c, func(err error) tea.Msg { return editFinishedMsg{err} })
		}
	case "y":
		if target, ok := m.selectedTarget(); ok {
			m.clip = &clipEntry{path: target, cut: false}
			m.setStatus(lvlInfo, "Yanked: %s", filepath.Base(target))
		}
	case "x":
		if target, ok := m.selectedTarget(); ok {
			m.clip = &clipEntry{path: target, cut: true}
			m.setStatus(lvlWarn, "Cut: %s", filepath.Base(target))
		}
	case "p":
		m.paste()
	case "c":
		if target, ok := m.selectedTarget(); ok {
			return m.openCopyMenu(target)
		}
	case "d":
		if target, ok := m.selectedTarget(); ok {
			m.confirmKind = confirmDelete
			m.confirmPath = target
			m.confirmMsg = fmt.Sprintf("Delete '%s'?", filepath.Base(target))
			m.mode = modeConfirm
		}
	case "r":
		if target, ok := m.selectedTarget(); ok {
			m.startPrompt(modeRename, "new name…", filepath.Base(target))
		}
	case "z":
		m.zip()
	case "s", "/":
		m.mode = modeFilter
		m.ti.SetValue("")
		m.ti.Prompt = "/ "
		m.applyView()
		return m, m.ti.Focus()
	case "f":
		return m.openPicker(pickFind)
	case "n":
		// Cycle: newest first → oldest first → original (name) order.
		switch m.sortMode {
		case sortName:
			m.sortMode = sortTimeDesc
		case sortTimeDesc:
			m.sortMode = sortTimeAsc
		default:
			m.sortMode = sortName
		}
		m.cursor, m.top = 0, 0
		m.reload()
	case "m":
		added, err := addBookmark(m.dir)
		if err != nil {
			m.setStatus(lvlErr, "bookmark: %v", err)
		} else if added {
			m.setStatus(lvlInfo, "Bookmarked: %s", m.dir)
		} else {
			m.setStatus(lvlInfo, "Already bookmarked")
		}
	case "b":
		return m.openPicker(pickBookmarks)
	case "t":
		// Toggle between name order and newest-first.
		if m.sortMode == sortName {
			m.sortMode = sortTimeDesc
		} else {
			m.sortMode = sortName
		}
		m.reload()
	case ".":
		m.showDots = !m.showDots
		m.reload()
	case "D":
		m.showDirs = !m.showDirs
		m.reload()
	case "?":
		m.showHelp = true
	}
	return m, nil
}

func (m *model) move(delta int) {
	m.cursor += delta
	m.clamp()
}

// selectedTarget returns the absolute path of the highlighted entry (never the
// ".." row).
func (m model) selectedTarget() (string, bool) {
	r, ok := m.current()
	if !ok || r.isParent {
		return "", false
	}
	return filepath.Join(m.dir, r.name), true
}

func (m model) openSelected() (tea.Model, tea.Cmd) {
	r, ok := m.current()
	if !ok {
		return m, nil
	}
	if r.isParent {
		m.goParent()
		return m, nil
	}
	target := filepath.Join(m.dir, r.name)
	info, err := os.Stat(target)
	if err == nil && info.IsDir() {
		m.enterDir(target)
		return m, nil
	}
	if err := openDetached("xdg-open", target); err != nil {
		m.setStatus(lvlErr, "xdg-open: %v", err)
	}
	return m, nil
}

func (m *model) startPrompt(md mode, placeholder, value string) {
	m.mode = md
	if md == modeRename {
		m.ti.Prompt = "rename: "
	} else {
		m.ti.Prompt = "open with: "
	}
	m.ti.SetValue(value)
	m.ti.CursorEnd()
	m.ti.Placeholder = placeholder
	m.ti.Focus()
}

func (m *model) paste() {
	if m.clip == nil {
		m.setStatus(lvlErr, "Nothing to paste")
		return
	}
	target := filepath.Join(m.dir, filepath.Base(m.clip.path))
	if _, err := os.Lstat(target); err == nil {
		m.confirmKind = confirmPaste
		m.confirmPath = target
		m.confirmMsg = fmt.Sprintf("Overwrite '%s'?", filepath.Base(target))
		m.mode = modeConfirm
		return
	}
	m.doPaste()
}

func (m *model) doPaste() {
	if m.clip == nil {
		return
	}
	target := filepath.Join(m.dir, filepath.Base(m.clip.path))
	name := filepath.Base(m.clip.path)
	if m.clip.cut {
		if err := movePath(m.clip.path, target); err != nil {
			m.setStatus(lvlErr, "move: %v", err)
			return
		}
		m.clip = nil
		m.setStatus(lvlInfo, "Moved: %s", name)
	} else {
		if err := copyPath(m.clip.path, target); err != nil {
			m.setStatus(lvlErr, "copy: %v", err)
			return
		}
		m.setStatus(lvlInfo, "Copied: %s", name)
	}
	m.reload()
	m.cursorTo(name)
}

func (m *model) zip() {
	target, ok := m.selectedTarget()
	if !ok {
		return
	}
	msg, err := runZip(target)
	if err != nil {
		m.setStatus(lvlErr, "zip: %v", err)
		return
	}
	m.reload()
	m.setStatus(lvlInfo, "%s", msg)
}

func (m model) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeBrowse
		m.ti.SetValue("")
		m.ti.Blur()
		m.applyView()
		return m, nil
	case "enter":
		m.mode = modeBrowse
		m.ti.Blur()
		md, cmd := m.openSelected()
		mm := md.(model)
		mm.ti.SetValue("")
		mm.applyView()
		return mm, cmd
	case "up", "ctrl+k", "ctrl+p":
		m.move(-1)
		return m, nil
	case "down", "ctrl+j", "ctrl+n":
		m.move(1)
		return m, nil
	}
	var cmd tea.Cmd
	m.ti, cmd = m.ti.Update(msg)
	m.cursor, m.top = 0, 0
	m.applyView()
	return m, cmd
}

func (m model) updatePrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeBrowse
		m.ti.Blur()
		return m, nil
	case "enter":
		val := strings.TrimSpace(m.ti.Value())
		md := m.mode
		m.mode = modeBrowse
		m.ti.Blur()
		if md == modeRename {
			m.applyRename(val)
		} else {
			m.applyOpenWith(val)
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.ti, cmd = m.ti.Update(msg)
	return m, cmd
}

func (m *model) applyRename(newName string) {
	target, ok := m.selectedTarget()
	if !ok {
		return
	}
	old := filepath.Base(target)
	if newName == "" || newName == old {
		return
	}
	dst := filepath.Join(m.dir, newName)
	if err := os.Rename(target, dst); err != nil {
		m.setStatus(lvlErr, "rename: %v", err)
		return
	}
	m.reload()
	m.cursorTo(newName)
	m.setStatus(lvlInfo, "Renamed to %s", newName)
}

func (m *model) applyOpenWith(cmdline string) {
	target, ok := m.selectedTarget()
	if !ok || cmdline == "" {
		return
	}
	fields := strings.Fields(cmdline)
	args := append(fields[1:], target)
	if err := openDetached(fields[0], args...); err != nil {
		m.setStatus(lvlErr, "%v", err)
	}
}

func (m model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "enter":
		m.mode = modeBrowse
		switch m.confirmKind {
		case confirmDelete:
			name := filepath.Base(m.confirmPath)
			if err := os.RemoveAll(m.confirmPath); err != nil {
				m.setStatus(lvlErr, "delete: %v", err)
			} else {
				m.reload()
				m.setStatus(lvlWarn, "Deleted: %s", name)
			}
		case confirmPaste:
			m.doPaste()
		}
		return m, nil
	case "n", "N", "esc":
		m.mode = modeBrowse
		return m, nil
	}
	return m, nil
}

// fuzzyMatch reports whether pattern's characters appear in s in order
// (case-insensitive subsequence match), like a lightweight fzf.
func fuzzyMatch(pattern, s string) bool {
	pattern = strings.ToLower(pattern)
	s = strings.ToLower(s)
	pi := 0
	for si := 0; si < len(s) && pi < len(pattern); si++ {
		if s[si] == pattern[pi] {
			pi++
		}
	}
	return pi == len(pattern)
}

func main() {
	start := "."
	if len(os.Args) > 1 {
		start = os.Args[1]
	}
	abs, err := filepath.Abs(start)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fe:", err)
		os.Exit(1)
	}
	if info, err := os.Stat(abs); err != nil || !info.IsDir() {
		fmt.Fprintln(os.Stderr, "fe: not a directory:", abs)
		os.Exit(1)
	}

	p := tea.NewProgram(newModel(abs), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "fe:", err)
		os.Exit(1)
	}
}
