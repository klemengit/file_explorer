package main

import (
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
)

// openPicker builds the item list for a find/newest/bookmark picker and enters
// picker mode. Bookmarks with no entries stay in browse mode with a notice.
func (m model) openPicker(kind pickerKind) (tea.Model, tea.Cmd) {
	var items []string
	var title string
	switch kind {
	case pickFind:
		items = deepFind(m.dir)
		title = "find"
	case pickRecent:
		items = recentFiles(m.dir, m.recentN)
		title = "newest files"
	case pickBookmarks:
		items = loadBookmarks()
		title = "bookmarks  ·  enter: go · ctrl-d: delete"
		if len(items) == 0 {
			m.setStatus(lvlWarn, "No bookmarks yet — press m to add one")
			return m, nil
		}
	}
	m.pickerKind = kind
	m.pickerTitle = title
	m.pickerAll = items
	m.pickerCursor = 0
	m.pickerTop = 0
	m.mode = modePicker
	m.ti.Prompt = "/ "
	m.ti.SetValue("")
	m.ti.Placeholder = ""
	m.pickerApplyFilter()
	return m, m.ti.Focus()
}

func (m *model) pickerApplyFilter() {
	q := m.ti.Value()
	m.pickerRows = m.pickerRows[:0]
	for i, it := range m.pickerAll {
		if q == "" || fuzzyMatch(q, it) {
			m.pickerRows = append(m.pickerRows, i)
		}
	}
	if m.pickerCursor >= len(m.pickerRows) {
		m.pickerCursor = len(m.pickerRows) - 1
	}
	if m.pickerCursor < 0 {
		m.pickerCursor = 0
	}
	m.pickerClampScroll()
}

func (m *model) pickerHeight() int {
	h := m.height - 3 // title + filter + hint
	if h < 1 {
		h = 1
	}
	return h
}

func (m *model) pickerClampScroll() {
	h := m.pickerHeight()
	if m.pickerCursor < m.pickerTop {
		m.pickerTop = m.pickerCursor
	}
	if m.pickerCursor >= m.pickerTop+h {
		m.pickerTop = m.pickerCursor - h + 1
	}
	if m.pickerTop < 0 {
		m.pickerTop = 0
	}
}

func (m model) updatePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeBrowse
		m.ti.SetValue("")
		m.ti.Blur()
		return m, nil
	case "up", "ctrl+k", "ctrl+p":
		m.pickerCursor--
		if m.pickerCursor < 0 {
			m.pickerCursor = 0
		}
		m.pickerClampScroll()
		return m, nil
	case "down", "ctrl+j", "ctrl+n":
		m.pickerCursor++
		if m.pickerCursor >= len(m.pickerRows) {
			m.pickerCursor = len(m.pickerRows) - 1
		}
		m.pickerClampScroll()
		return m, nil
	case "ctrl+d":
		if m.pickerKind == pickBookmarks {
			if idx, ok := m.pickerSelectedIndex(); ok {
				removeBookmark(m.pickerAll[idx])
				m.pickerAll = loadBookmarks()
				m.pickerApplyFilter()
			}
		}
		return m, nil
	case "enter":
		return m.pickerSelect()
	}
	var cmd tea.Cmd
	m.ti, cmd = m.ti.Update(msg)
	m.pickerCursor, m.pickerTop = 0, 0
	m.pickerApplyFilter()
	return m, cmd
}

func (m model) pickerSelectedIndex() (int, bool) {
	if m.pickerCursor < 0 || m.pickerCursor >= len(m.pickerRows) {
		return 0, false
	}
	return m.pickerRows[m.pickerCursor], true
}

func (m model) pickerSelect() (tea.Model, tea.Cmd) {
	idx, ok := m.pickerSelectedIndex()
	if !ok {
		return m, nil
	}
	item := m.pickerAll[idx]
	m.mode = modeBrowse
	m.ti.SetValue("")
	m.ti.Blur()

	switch m.pickerKind {
	case pickFind:
		target := filepath.Join(m.dir, item)
		if info, err := os.Stat(target); err == nil && info.IsDir() {
			m.enterDir(target)
		} else {
			m.enterDir(filepath.Dir(target))
			m.cursorTo(filepath.Base(target))
		}
	case pickRecent:
		target := filepath.Join(m.dir, item)
		m.enterDir(filepath.Dir(target))
		m.cursorTo(filepath.Base(target))
	case pickBookmarks:
		if info, err := os.Stat(item); err == nil && info.IsDir() {
			m.enterDir(item)
		} else {
			m.setStatus(lvlErr, "No longer exists: %s", item)
		}
	case pickOpenWith:
		// The trailing item past the app list is "Custom command…".
		if idx >= len(m.openWith) {
			m.startPrompt(modeOpenWith, "command…", "")
			return m, m.ti.Focus()
		}
		cmd := m.launchApp(m.openWith[idx], m.openWithTarget)
		return m, cmd
	case pickCopy:
		m.applyCopy(idx)
		return m, nil
	}
	return m, nil
}
