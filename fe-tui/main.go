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
	modeArchive
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

	// Multi-selection. marked holds the names of entries picked with space (or
	// committed from a visual range); visual/anchor describe a live vim-style
	// linewise selection that follows the cursor until it is committed or
	// cancelled. Both are per-directory and reset when the pane moves.
	marked map[string]bool
	visual bool
	anchor int
}

type model struct {
	panes  [2]pane
	active int // 0 = left, 1 = right

	width  int
	height int

	mode mode
	ti   textinput.Model

	confirmKind  confirmKind
	confirmPaths []string
	confirmMsg   string

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

	openWith        []app
	openWithTargets []string

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
	p.clearSelection() // marks name entries of the old directory
	return p.reload(filter)
}

// visualRange returns the inclusive row range covered by a live visual
// selection, ordered low→high. ok is false when visual mode is off.
func (p pane) visualRange() (lo, hi int, ok bool) {
	if !p.visual {
		return 0, 0, false
	}
	lo, hi = p.anchor, p.cursor
	if lo > hi {
		lo, hi = hi, lo
	}
	if lo < 0 {
		lo = 0
	}
	if hi >= len(p.rows) {
		hi = len(p.rows) - 1
	}
	return lo, hi, lo <= hi
}

// isSelected reports whether row i is part of the current selection — either
// explicitly marked or inside the live visual range. ".." is never selectable.
func (p pane) isSelected(i int) bool {
	if i < 0 || i >= len(p.rows) || p.rows[i].isParent {
		return false
	}
	if lo, hi, ok := p.visualRange(); ok && i >= lo && i <= hi {
		return true
	}
	return p.marked[p.rows[i].name]
}

// selectedPaths returns the absolute paths of every selected row, in display
// order, or nil when nothing is selected. Only visible rows count, so a mark
// hidden by the current filter is left alone rather than acted on unseen.
func (p pane) selectedPaths() []string {
	var out []string
	for i := range p.rows {
		if p.isSelected(i) {
			out = append(out, filepath.Join(p.dir, p.rows[i].name))
		}
	}
	return out
}

// targets is what an action should operate on: the whole selection when there
// is one, otherwise just the row under the cursor. Every bulk-capable command
// goes through this, so single-item use is unchanged when nothing is selected.
func (p pane) targets() []string {
	if sel := p.selectedPaths(); len(sel) > 0 {
		return sel
	}
	if t, ok := p.selectedTarget(); ok {
		return []string{t}
	}
	return nil
}

// toggleMark flips the mark on the row under the cursor (never "..").
func (p *pane) toggleMark() {
	r, ok := p.current()
	if !ok || r.isParent {
		return
	}
	if p.marked == nil {
		p.marked = make(map[string]bool)
	}
	if p.marked[r.name] {
		delete(p.marked, r.name)
	} else {
		p.marked[r.name] = true
	}
}

// commitVisual folds the live visual range into the marked set, then leaves
// visual mode.
func (p *pane) commitVisual() {
	lo, hi, ok := p.visualRange()
	p.visual = false
	if !ok {
		return
	}
	if p.marked == nil {
		p.marked = make(map[string]bool)
	}
	for i := lo; i <= hi; i++ {
		if !p.rows[i].isParent {
			p.marked[p.rows[i].name] = true
		}
	}
}

// clearSelection drops every mark and leaves visual mode.
func (p *pane) clearSelection() {
	p.marked = nil
	p.visual = false
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
		case modeRename, modeOpenWith, modeArchive:
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
	case "V":
		// Vim-style linewise visual selection: V starts it at the cursor, j/k
		// extend it, and a second V commits the range to the selection.
		if p.visual {
			p.commitVisual()
			m.setStatus(lvlInfo, "%d selected", len(p.selectedPaths()))
		} else {
			p.visual = true
			p.anchor = p.cursor
		}
	case " ", "space":
		// Mark the row and step down, so tapping space runs down a block.
		p.toggleMark()
		p.move(1)
	case "esc":
		// First esc drops a live visual range, a second clears the marks.
		if p.visual {
			p.visual = false
		} else if len(p.marked) > 0 {
			p.clearSelection()
		}
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
		if targets := p.targets(); len(targets) > 0 {
			return m.openOpenWith(targets)
		}
	case "e":
		if targets := p.targets(); len(targets) > 0 {
			c := exec.Command("nvim", targets...)
			return m, tea.ExecProcess(c, func(err error) tea.Msg { return editFinishedMsg{err} })
		}
	case "y":
		if targets := p.targets(); len(targets) > 0 {
			m.clip = &clipEntry{paths: targets, cut: false}
			m.setStatus(lvlInfo, "Yanked: %s", describePaths(targets))
			p.clearSelection()
		}
	case "x":
		if targets := p.targets(); len(targets) > 0 {
			m.clip = &clipEntry{paths: targets, cut: true}
			m.setStatus(lvlWarn, "Cut: %s", describePaths(targets))
			p.clearSelection()
		}
	case "p":
		m.paste()
	case "c":
		if targets := p.targets(); len(targets) > 0 {
			return m.openCopyMenu(targets)
		}
	case "d":
		if targets := p.targets(); len(targets) > 0 {
			m.confirmKind = confirmDelete
			m.confirmPaths = targets
			m.confirmMsg = fmt.Sprintf("Delete %s?", describeQuoted(targets))
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

// transfer copies (move=false) or moves (move=true) the active pane's targets —
// the whole selection, or the row under the cursor — into the other pane's
// directory. A failure stops the run and reports the entry that failed.
func (m *model) transfer(move bool) {
	srcs := m.cur().targets()
	if len(srcs) == 0 {
		return
	}
	dstDir := m.other().dir
	if dstDir == m.cur().dir {
		m.setStatus(lvlWarn, "Both panes show the same directory")
		return
	}
	verb, failed := "copy", false
	if move {
		verb = "move"
	}
	var last string
	for _, src := range srcs {
		name := filepath.Base(src)
		dst := filepath.Join(dstDir, name)
		var err error
		if move {
			err = movePath(src, dst)
		} else {
			err = copyPath(src, dst)
		}
		if err != nil {
			m.setStatus(lvlErr, "%s %s: %v", verb, name, err)
			failed = true
			break
		}
		last = name
	}
	// Refresh both panes; put the other pane's cursor on the last new entry.
	// A failed run keeps the selection so it can be retried or trimmed.
	if !failed {
		m.cur().clearSelection()
	}
	m.cur().reload(m.filterQuery())
	m.other().reload("")
	m.other().cursorTo(last)
	if !failed {
		done := "Copied"
		if move {
			done = "Moved"
		}
		m.setStatus(lvlInfo, "%s %s → %s", done, describePaths(srcs), abbrevHome(dstDir))
	}
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
	switch md {
	case modeRename:
		m.ti.Prompt = "rename: "
	case modeArchive:
		m.ti.Prompt = "zip as: "
	default:
		m.ti.Prompt = "open with: "
	}
	m.ti.SetValue(value)
	m.ti.CursorEnd()
	m.ti.Placeholder = placeholder
	m.ti.Focus()
}

// paste drops the clipboard into the active pane's directory, asking once up
// front if any of the entries would overwrite something already there.
func (m *model) paste() {
	if m.clip == nil || len(m.clip.paths) == 0 {
		m.setStatus(lvlErr, "Nothing to paste")
		return
	}
	var clashes []string
	for _, src := range m.clip.paths {
		name := filepath.Base(src)
		if _, err := os.Lstat(filepath.Join(m.cur().dir, name)); err == nil {
			clashes = append(clashes, name)
		}
	}
	if len(clashes) > 0 {
		m.confirmKind = confirmPaste
		m.confirmMsg = fmt.Sprintf("Overwrite '%s'?", clashes[0])
		if len(clashes) > 1 {
			m.confirmMsg = fmt.Sprintf("Overwrite %d existing entries?", len(clashes))
		}
		m.mode = modeConfirm
		return
	}
	m.doPaste()
}

func (m *model) doPaste() {
	if m.clip == nil {
		return
	}
	paths, cut := m.clip.paths, m.clip.cut
	var last string
	done := 0
	for _, src := range paths {
		name := filepath.Base(src)
		dst := filepath.Join(m.cur().dir, name)
		var err error
		if cut {
			err = movePath(src, dst)
		} else {
			err = copyPath(src, dst)
		}
		if err != nil {
			m.setStatus(lvlErr, "paste %s: %v", name, err)
			break
		}
		done++
		last = name
	}
	if cut {
		// Drop what actually moved; anything left still exists at its source.
		m.clip.paths = paths[done:]
		if len(m.clip.paths) == 0 {
			m.clip = nil
		}
	}
	m.reload()
	m.cur().cursorTo(last)
	if done == len(paths) {
		verb := "Copied"
		if cut {
			verb = "Moved"
		}
		m.setStatus(lvlInfo, "%s: %s", verb, describePaths(paths))
	}
}

// zip archives the active pane's targets. A single entry keeps the old
// behaviour (foo → foo.zip, or unzip a .zip); several entries go into one
// archive, so it asks for a name first.
func (m *model) zip() {
	targets := m.cur().targets()
	switch {
	case len(targets) == 0:
		return
	case len(targets) == 1:
		msg, err := runZip(targets[0])
		if err != nil {
			m.setStatus(lvlErr, "zip: %v", err)
			return
		}
		m.cur().clearSelection()
		m.reload()
		m.setStatus(lvlInfo, "%s", msg)
	default:
		m.startPrompt(modeArchive, "archive name…", filepath.Base(m.cur().dir)+".zip")
	}
}

// applyArchive zips the current selection into one archive named by the prompt.
func (m *model) applyArchive(archive string) {
	targets := m.cur().targets()
	if archive == "" || len(targets) == 0 {
		return
	}
	if !strings.HasSuffix(archive, ".zip") {
		archive += ".zip"
	}
	names := make([]string, len(targets))
	for i, t := range targets {
		names[i] = filepath.Base(t)
	}
	if err := zipInto(m.cur().dir, archive, names); err != nil {
		m.setStatus(lvlErr, "zip: %v", err)
		return
	}
	m.cur().clearSelection()
	m.reload()
	m.cur().cursorTo(archive)
	m.setStatus(lvlInfo, "Zipped %d entries → %s", len(names), archive)
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
		switch md {
		case modeRename:
			m.applyRename(val)
		case modeArchive:
			m.applyArchive(val)
		default:
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
	targets := m.cur().targets()
	if len(targets) == 0 || cmdline == "" {
		return
	}
	fields := strings.Fields(cmdline)
	args := append(fields[1:], targets...)
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
			done := 0
			for _, path := range m.confirmPaths {
				if err := os.RemoveAll(path); err != nil {
					m.setStatus(lvlErr, "delete %s: %v", filepath.Base(path), err)
					break
				}
				done++
			}
			// Whatever was deleted drops out of the listing on reload, so a
			// partial run leaves exactly the survivors selected.
			if done == len(m.confirmPaths) {
				m.cur().clearSelection()
			}
			m.reload()
			if done == len(m.confirmPaths) {
				m.setStatus(lvlWarn, "Deleted: %s", describePaths(m.confirmPaths))
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

// describePaths names a batch for a status line: the entry's own name when
// there is one, "N entries" otherwise.
func describePaths(paths []string) string {
	if len(paths) == 1 {
		return filepath.Base(paths[0])
	}
	return fmt.Sprintf("%d entries", len(paths))
}

// describeQuoted is describePaths for prompts, where a lone name reads better
// in quotes ("Delete 'notes.md'?" vs "Delete 3 entries?").
func describeQuoted(paths []string) string {
	if len(paths) == 1 {
		return "'" + filepath.Base(paths[0]) + "'"
	}
	return fmt.Sprintf("%d entries", len(paths))
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
