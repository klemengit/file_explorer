package main

import (
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// fe does not watch the filesystem; it looks. Once a second each pane stats its
// own directory and re-reads it only when the mtime has moved, which is what
// creating, deleting or renaming an entry does to it. Sitting idle that is two
// stat calls a second — cheaper than carrying an inotify watch per pane, and it
// behaves the same on a network mount, where inotify quietly delivers nothing.
//
// A directory's mtime does not move when a file already inside it is written
// to, so a download filling up would keep the size it had when it started.
// Every fullEvery-th tick the panes are re-read whether or not they look
// changed, which costs one listing every few seconds and keeps the size and
// date columns honest.
const (
	refreshInterval = time.Second
	fullEvery       = 5
)

// refreshTickMsg is the poll. It carries nothing: which panes are stale is a
// question for the moment it arrives, not for the moment it was scheduled.
type refreshTickMsg struct{}

func refreshTick() tea.Cmd {
	return tea.Tick(refreshInterval, func(time.Time) tea.Msg { return refreshTickMsg{} })
}

// dirStamp is a directory's modification time, or the zero time when it cannot
// be read at all. A pane whose directory was deleted or unmounted stamps as
// zero, matches the zero its failed read stored, and so stops asking to be
// re-read every second for as long as it stays gone.
func dirStamp(dir string) time.Time {
	info, err := os.Stat(dir)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

// autoRefresh re-reads whichever panes have changed underneath fe. force skips
// the mtime check and re-reads both regardless, which is what the poll does on
// its slow beat and what ctrl-r and switching panes do on demand.
func (m *model) autoRefresh(force bool) {
	for i := range m.panes {
		p := &m.panes[i]
		if p.visual {
			// A live visual range is anchored to a row number, so a listing
			// that grew or shrank would leave it selecting different files
			// than the ones highlighted when it was drawn.
			continue
		}
		if !force && dirStamp(p.dir).Equal(p.stamp) {
			continue
		}
		p.refresh("")
	}
}

// refresh re-reads the pane in place. Unlike reload it keeps the cursor on the
// entry it was on rather than on the row number it was on: a file arriving
// above the cursor must not drag the selection down a line with it. The
// cursor's distance from the top of the pane is kept as well, so the rows
// scroll underneath a highlight that stays where the eye left it.
//
// When the entry under the cursor is the one that disappeared there is nothing
// to go back to, and the row number turns out to be the right answer after
// all — the cursor lands on whatever moved up into the gap.
func (p *pane) refresh(filter string) error {
	var name string
	if r, ok := p.current(); ok && !r.isParent {
		name = r.name
	}
	offset := p.cursor - p.top

	err := p.reload(filter)

	if name != "" {
		p.cursorTo(name)
	}
	p.top = p.cursor - offset
	// A listing that shrank a long way can leave the old offset pointing past
	// the end, which would draw the last few entries against a screen of blank
	// rows. Scrolling stops where the list does.
	if maxTop := len(p.rows) - p.listHeight(); p.top > maxTop {
		p.top = maxTop
	}
	if p.top < 0 {
		p.top = 0
	}
	p.clampScroll()
	return err
}
