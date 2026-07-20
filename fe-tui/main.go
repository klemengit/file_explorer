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

// pane holds the browse state for one directory view: its listing, cursor,
// scroll offset, sort/visibility flags, and its own render box (width/height).
// The app always shows two panes side by side, commander-style.
type pane struct {
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
}

type model struct {
	panes  [2]pane
	active int // 0 = left, 1 = right

	width  int
	height int

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
	m := model{ti: ti}
	for i := range m.panes {
		m.panes[i] = pane{
			dir:      dir,
			showDirs: true,
			showDots: false,
			sortMode: sortName,
		}
		m.panes[i].reload("")
	}
	return m
}

// cur returns the active pane; other returns the inactive one.
func (m *model) cur() *pane   { return &m.panes[m.active] }
func (m *model) other() *pane { return &m.panes[1-m.active] }

// filterQuery is the live filter text, but only while in filter mode; otherwise
// the empty string (no filtering). The filter always targets the active pane.
func (m *model) filterQuery() string {
	if m.mode == modeFilter {
		return m.ti.Value()
	}
	return ""
}

func (m *model) setStatus(lv int, format string, a ...any) {
	m.status = fmt.Sprintf(format, a...)
	m.statusLv = lv
}

// reload re-reads the active pane's directory (surfacing read errors as status).
func (m *model) reload() {
	if err := m.cur().reload(m.filterQuery()); err != nil {
		m.setStatus(lvlErr, "Cannot read: %v", err)
	}
}

// applyView recomputes the active pane's visible rows for the current filter.
func (m *model) applyView() {
	m.cur().applyView(m.filterQuery())
}

// enterDir descends the active pane into path.
func (m *model) enterDir(path string) {
	if err := m.cur().enterDir(path, m.filterQuery()); err != nil {
		m.setStatus(lvlErr, "Cannot read: %v", err)
	}
}

// goParent moves the active pane up to its parent directory.
func (m *model) goParent() {
	if err := m.cur().goParent(m.filterQuery()); err != nil {
		m.setStatus(lvlErr, "Cannot read: %v", err)
	}
}

// reload re-reads the pane's directory and rebuilds its row list, applying the
// given filter query ("" = no filter). Returns any read error.
func (p *pane) reload(filter string) error {
	entries, err := listDir(p.dir, p.sortMode, p.showDirs, p.showDots)
	if err != nil {
		entries = nil
	}
	p.allRows = make([]row, 0, len(entries)+1)
	p.allRows = append(p.allRows, row{label: "..", isParent: true})
	for _, e := range entries {
		label := e.name
		if e.isDir {
			label += "/"
		}
		p.allRows = append(p.allRows, row{
			label:   label,
			name:    e.name,
			isDir:   e.isDir,
			isLink:  e.isLink,
			size:    e.size,
			modTime: e.modTime,
		})
	}
	p.applyView(filter)
	return err
}

// applyView recomputes p.rows from p.allRows, keeping only entries that match
// the filter query (empty query = show everything), then clamps cursor/scroll.
func (p *pane) applyView(filter string) {
	if filter != "" {
		var kept []row
		for _, r := range p.allRows {
			if r.isParent {
				continue
			}
			if fuzzyMatch(filter, r.name) {
				kept = append(kept, r)
			}
		}
		p.rows = kept
	} else {
		p.rows = p.allRows
	}
	p.clamp()
}

func (p *pane) clamp() {
	if p.cursor >= len(p.rows) {
		p.cursor = len(p.rows) - 1
	}
	if p.cursor < 0 {
		p.cursor = 0
	}
	p.clampScroll()
}

func (p *pane) clampScroll() {
	h := p.listHeight()
	if p.cursor < p.top {
		p.top = p.cursor
	}
	if p.cursor >= p.top+h {
		p.top = p.cursor - h + 1
	}
	if p.top < 0 {
		p.top = 0
	}
}

func (p *pane) listHeight() int {
	h := p.height - 2 // header + footer
	if h < 1 {
		h = 1
	}
	return h
}

func (p *pane) cursorTo(name string) {
	for i, r := range p.rows {
		if r.name == name {
			p.cursor = i
			p.clampScroll()
			return
		}
	}
}

func (p *pane) enterDir(path, filter string) error {
	p.dir = path
	p.cursor = 0
	p.top = 0
	return p.reload(filter)
}

func (p *pane) goParent(filter string) error {
	parent := filepath.Dir(p.dir)
	if parent == p.dir {
		return nil
	}
	child := filepath.Base(p.dir)
	err := p.enterDir(parent, filter)
	p.cursorTo(child)
	return err
}

func (p *pane) current() (row, bool) {
	if p.cursor < 0 || p.cursor >= len(p.rows) {
		return row{}, false
	}
	return p.rows[p.cursor], true
}

func (p *pane) move(delta int) {
	p.cursor += delta
	p.clamp()
}

// selectedTarget returns the absolute path of the highlighted entry (never the
// ".." parent row).
func (p pane) selectedTarget() (string, bool) {
	r, ok := p.current()
	if !ok || r.isParent {
		return "", false
	}
	return filepath.Join(p.dir, r.name), true
}

func (m model) Init() tea.Cmd { return nil }

// layout recomputes each pane's render box from the terminal size and the
// the terminal size, then re-clamps both panes' scroll offsets.
func (m *model) layout() {
	leftOuter := m.width / 2
	rightOuter := m.width - leftOuter
	// Each pane is drawn inside a 1-cell border, so its content width is the
	// box width minus the two border columns.
	m.panes[0].width = leftOuter - 2
	m.panes[1].width = rightOuter - 2
	if m.panes[0].width < 1 {
		m.panes[0].width = 1
	}
	if m.panes[1].width < 1 {
		m.panes[1].width = 1
	}
	// Two border rows plus the shared footer line are taken off the height.
	m.panes[0].height = m.height - 2
	m.panes[1].height = m.height - 2
	m.panes[0].clampScroll()
	m.panes[1].clampScroll()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ti.Width = msg.Width - 4
		m.layout()
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

	p := m.cur()

	switch key {
	case "q":
		return m, tea.Quit
	case "tab":
		// Switch the active pane between left and right.
		m.active = 1 - m.active
	case "f5":
		m.transfer(false)
	case "f6":
		m.transfer(true)
	case "j", "down":
		p.move(1)
	case "k", "up":
		p.move(-1)
	case "ctrl+d":
		p.move(p.listHeight() / 2)
	case "ctrl+u":
		p.move(-p.listHeight() / 2)
	case "pgdown":
		p.move(p.listHeight())
	case "pgup":
		p.move(-p.listHeight())
	case "g":
		if m.pendingG {
			p.cursor, p.top = 0, 0
			m.pendingG = false
		} else {
			m.pendingG = true
		}
	case "G":
		p.cursor = len(p.rows) - 1
		p.clampScroll()
	case "h", "left":
		m.goParent()
	case "l", "right", "enter":
		return m.openSelected()
	case "E":
		if err := openDetached("xdg-open", p.dir); err != nil {
			m.setStatus(lvlErr, "xdg-open: %v", err)
		}
	case "O":
		if target, ok := p.selectedTarget(); ok {
			return m.openOpenWith(target)
		}
	case "e":
		if target, ok := p.selectedTarget(); ok {
			c := exec.Command("nvim", target)
			return m, tea.ExecProcess(c, func(err error) tea.Msg { return editFinishedMsg{err} })
		}
	case "y":
		if target, ok := p.selectedTarget(); ok {
			m.clip = &clipEntry{path: target, cut: false}
			m.setStatus(lvlInfo, "Yanked: %s", filepath.Base(target))
		}
	case "x":
		if target, ok := p.selectedTarget(); ok {
			m.clip = &clipEntry{path: target, cut: true}
			m.setStatus(lvlWarn, "Cut: %s", filepath.Base(target))
		}
	case "p":
		m.paste()
	case "c":
		if target, ok := p.selectedTarget(); ok {
			return m.openCopyMenu(target)
		}
	case "d":
		if target, ok := p.selectedTarget(); ok {
			m.confirmKind = confirmDelete
			m.confirmPath = target
			m.confirmMsg = fmt.Sprintf("Delete '%s'?", filepath.Base(target))
			m.mode = modeConfirm
		}
	case "r":
		if target, ok := p.selectedTarget(); ok {
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
		switch p.sortMode {
		case sortName:
			p.sortMode = sortTimeDesc
		case sortTimeDesc:
			p.sortMode = sortTimeAsc
		default:
			p.sortMode = sortName
		}
		p.cursor, p.top = 0, 0
		m.reload()
	case "m":
		added, err := addBookmark(p.dir)
		if err != nil {
			m.setStatus(lvlErr, "bookmark: %v", err)
		} else if added {
			m.setStatus(lvlInfo, "Bookmarked: %s", p.dir)
		} else {
			m.setStatus(lvlInfo, "Already bookmarked")
		}
	case "b":
		return m.openPicker(pickBookmarks)
	case "t":
		// Toggle between name order and newest-first.
		if p.sortMode == sortName {
			p.sortMode = sortTimeDesc
		} else {
			p.sortMode = sortName
		}
		m.reload()
	case ".":
		p.showDots = !p.showDots
		m.reload()
	case "D":
		p.showDirs = !p.showDirs
		m.reload()
	case "?":
		m.showHelp = true
	}
	return m, nil
}

// transfer copies (move=false) or moves (move=true) the active pane's selected
// entry into the other pane's directory.
func (m *model) transfer(move bool) {
	src, ok := m.cur().selectedTarget()
	if !ok {
		return
	}
	dstDir := m.other().dir
	if dstDir == m.cur().dir {
		m.setStatus(lvlWarn, "Both panes show the same directory")
		return
	}
	name := filepath.Base(src)
	dst := filepath.Join(dstDir, name)
	if move {
		if err := movePath(src, dst); err != nil {
			m.setStatus(lvlErr, "move: %v", err)
			return
		}
	} else {
		if err := copyPath(src, dst); err != nil {
			m.setStatus(lvlErr, "copy: %v", err)
			return
		}
	}
	// Refresh both panes; put the other pane's cursor on the new entry.
	m.cur().reload(m.filterQuery())
	m.other().reload("")
	m.other().cursorTo(name)
	verb := "Copied"
	if move {
		verb = "Moved"
	}
	m.setStatus(lvlInfo, "%s %s → %s", verb, name, abbrevHome(dstDir))
}

func (m model) openSelected() (tea.Model, tea.Cmd) {
	p := m.cur()
	r, ok := p.current()
	if !ok {
		return m, nil
	}
	if r.isParent {
		m.goParent()
		return m, nil
	}
	target := filepath.Join(p.dir, r.name)
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
	target := filepath.Join(m.cur().dir, filepath.Base(m.clip.path))
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
	target := filepath.Join(m.cur().dir, filepath.Base(m.clip.path))
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
	m.cur().cursorTo(name)
}

func (m *model) zip() {
	target, ok := m.cur().selectedTarget()
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
		m.cur().move(-1)
		return m, nil
	case "down", "ctrl+j", "ctrl+n":
		m.cur().move(1)
		return m, nil
	}
	var cmd tea.Cmd
	m.ti, cmd = m.ti.Update(msg)
	m.cur().cursor, m.cur().top = 0, 0
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
	target, ok := m.cur().selectedTarget()
	if !ok {
		return
	}
	old := filepath.Base(target)
	if newName == "" || newName == old {
		return
	}
	dst := filepath.Join(m.cur().dir, newName)
	if err := os.Rename(target, dst); err != nil {
		m.setStatus(lvlErr, "rename: %v", err)
		return
	}
	m.reload()
	m.cur().cursorTo(newName)
	m.setStatus(lvlInfo, "Renamed to %s", newName)
}

func (m *model) applyOpenWith(cmdline string) {
	target, ok := m.cur().selectedTarget()
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
