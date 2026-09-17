package main

import (
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// A directory has no size of its own worth showing — the number stat gives back
// is the size of the entry list, not of what is in it. The only honest answer
// is the one `du` gives: walk the whole tree and add the files up. That takes
// however long it takes, so it happens in its own goroutine and the properties
// window redraws from it while it runs, counting up. Closing the window stops
// the walk.

// propsScanInterval is how often the window redraws while a walk is running.
// Fast enough that the numbers visibly climb, slow enough that a directory of
// small files isn't spending its time on redraws.
const propsScanInterval = 120 * time.Millisecond

// propsTickMsg redraws the properties window mid-walk. Like the refresh poll it
// carries nothing: what the numbers are is a question for the moment it lands.
type propsTickMsg struct{}

func propsTick() tea.Cmd {
	return tea.Tick(propsScanInterval, func(time.Time) tea.Msg { return propsTickMsg{} })
}

// dirScan is a recursive walk in progress, shared between the goroutine filling
// it in and the render that reads it. Everything behind the mutex is written by
// the walk and read by the window, so both sides take it.
type dirScan struct {
	mu      sync.Mutex
	bytes   int64
	files   int64
	dirs    int64
	skipped int64 // entries that could not be read at all

	// children charges every byte to the entry directly under root it sits
	// under, which is the "where did the space actually go" answer. dirChildren
	// remembers which of those entries are themselves directories, so the
	// window can mark them.
	children    map[string]int64
	dirChildren map[string]bool

	done   bool
	cancel chan struct{}
}

// scanState is a copy of a walk's numbers, taken under the lock so the render
// works from one consistent moment rather than from fields that move under it.
type scanState struct {
	bytes, files, dirs, skipped int64
	done                        bool
	largest                     []childSize
}

// childSize is one entry directly under the scanned directory, with everything
// below it added up — the "where did the space actually go" answer.
type childSize struct {
	name  string
	bytes int64
	isDir bool
}

// startScan walks dir in the background and returns the scan to read it from.
func startScan(dir string) *dirScan {
	s := &dirScan{
		children:    make(map[string]int64),
		dirChildren: make(map[string]bool),
		cancel:      make(chan struct{}),
	}
	go s.walk(dir)
	return s
}

// stop ends the walk early. It is safe to call more than once, since closing an
// already-closed channel panics and a window can be shut once.
func (s *dirScan) stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-s.cancel:
	default:
		close(s.cancel)
	}
}

// stopped reports whether the window has gone away and the walk should give up.
func (s *dirScan) stopped() bool {
	select {
	case <-s.cancel:
		return true
	default:
		return false
	}
}

// walk adds up every file under root. Symlinks are counted at their own size
// and never followed: a link into a parent directory would otherwise send the
// walk round in circles, and a link out of the tree would charge this directory
// for bytes that live somewhere else. Dotfiles are counted whether or not the
// pane is showing them — they take up the same room either way.
func (s *dirScan) walk(root string) {
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if s.stopped() {
			return filepath.SkipAll
		}
		if err != nil {
			s.addSkipped()
			if d != nil && d.IsDir() {
				return filepath.SkipDir // unreadable directory: count it, move on
			}
			return nil
		}
		if path == root {
			return nil
		}
		top := topChild(root, path)
		if d.IsDir() {
			s.addDir(top, top == d.Name())
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			s.addSkipped()
			return nil
		}
		s.addFile(info.Size(), top)
		return nil
	})

	s.mu.Lock()
	s.done = true
	s.mu.Unlock()
}

// addFile folds one file into the totals and charges its bytes to the entry
// directly under root that it sits in (or is).
func (s *dirScan) addFile(bytes int64, top string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files++
	s.bytes += bytes
	if top != "" {
		s.children[top] += bytes
	}
}

// addDir counts a subdirectory. direct says it is one of root's own children,
// which is the only case the largest-children list needs to know about.
func (s *dirScan) addDir(top string, direct bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dirs++
	if direct && top != "" {
		s.dirChildren[top] = true
		if _, seen := s.children[top]; !seen {
			s.children[top] = 0 // an empty subdirectory is still worth listing
		}
	}
}

// addSkipped counts an entry that could not be read, so the window can say the
// total is a floor rather than an answer.
func (s *dirScan) addSkipped() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.skipped++
}

// propsLargest is how many of the biggest children the window lists.
const propsLargest = 5

// state takes a consistent snapshot of the walk, including its biggest children
// so far, ordered largest first.
func (s *dirScan) state() scanState {
	s.mu.Lock()
	defer s.mu.Unlock()

	st := scanState{bytes: s.bytes, files: s.files, dirs: s.dirs, skipped: s.skipped, done: s.done}
	for name, b := range s.children {
		st.largest = append(st.largest, childSize{name: name, bytes: b, isDir: s.dirChildren[name]})
	}
	sort.Slice(st.largest, func(i, j int) bool {
		if st.largest[i].bytes != st.largest[j].bytes {
			return st.largest[i].bytes > st.largest[j].bytes
		}
		return st.largest[i].name < st.largest[j].name
	})
	if len(st.largest) > propsLargest {
		st.largest = st.largest[:propsLargest]
	}
	return st
}

// topChild names the entry directly under root that path is, or sits inside.
func topChild(root, path string) string {
	rest := strings.TrimPrefix(path, root)
	rest = strings.TrimPrefix(rest, string(filepath.Separator))
	if i := strings.IndexByte(rest, filepath.Separator); i >= 0 {
		rest = rest[:i]
	}
	return rest
}

// props is what the properties window shows: the facts one stat answers
// outright, plus — for a directory — the walk still adding up what is inside.
type props struct {
	path    string
	name    string
	isDir   bool
	isLink  bool
	link    string // where a symlink points, unresolved
	mode    fs.FileMode
	modTime time.Time
	size    int64 // the entry's own size, as lstat reports it
	owner   string

	scan *dirScan // nil unless this is a real directory
}

// statProps reads everything about path that does not need a walk. It does not
// follow symlinks: a link's properties are the link's, and the target is named
// on its own line.
func statProps(path string) (*props, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	p := &props{
		path:    path,
		name:    filepath.Base(path),
		isLink:  info.Mode()&fs.ModeSymlink != 0,
		isDir:   info.IsDir(),
		mode:    info.Mode(),
		modTime: info.ModTime(),
		size:    info.Size(),
		owner:   ownerOf(info),
	}
	if p.isLink {
		if t, lerr := os.Readlink(path); lerr == nil {
			p.link = t
		}
	}
	if p.isDir {
		p.scan = startScan(path)
	}
	return p, nil
}

// ownerOf names the user and group owning the entry, falling back to the raw
// numbers when they belong to no account this machine knows about.
func ownerOf(info os.FileInfo) string {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return ""
	}
	uid, gid := strconv.FormatUint(uint64(st.Uid), 10), strconv.FormatUint(uint64(st.Gid), 10)
	name, group := uid, gid
	if u, err := user.LookupId(uid); err == nil {
		name = u.Username
	}
	if g, err := user.LookupGroupId(gid); err == nil {
		group = g.Name
	}
	return name + ":" + group
}

// kind names what the entry is, in the words the window uses.
func (p *props) kind() string {
	switch {
	case p.isLink:
		return "symlink"
	case p.isDir:
		return "directory"
	case p.mode&fs.ModeDevice != 0:
		return "device"
	case p.mode&fs.ModeNamedPipe != 0:
		return "pipe"
	case p.mode&fs.ModeSocket != 0:
		return "socket"
	default:
		return "file"
	}
}

// openProps opens the properties window on the entry under the cursor, or on
// the pane's own directory when the cursor is on "..". Sitting on ".." and
// asking for properties is how you get the size of the directory you are in.
func (m model) openProps() (tea.Model, tea.Cmd) {
	p := m.cur()
	path := p.dir
	if r, ok := p.current(); ok && !r.isParent {
		path = filepath.Join(p.dir, r.name)
	}
	pr, err := statProps(path)
	if err != nil {
		m.setStatus(lvlErr, "properties: %v", err)
		return m, nil
	}
	m.props = pr
	m.mode = modeProps
	if pr.scan != nil {
		return m, propsTick()
	}
	return m, nil
}

// closeProps shuts the window and stops any walk it started.
func (m *model) closeProps() {
	if m.props != nil && m.props.scan != nil {
		m.props.scan.stop()
	}
	m.props = nil
	m.mode = modeBrowse
}

func (m model) updateProps(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "s", "enter":
		m.closeProps()
	}
	return m, nil
}

// commas groups a number in threes, so a byte count reads at a glance instead
// of having to be counted off with a finger.
func commas(n int64) string {
	s := strconv.FormatInt(n, 10)
	sign := ""
	if strings.HasPrefix(s, "-") {
		sign, s = "-", s[1:]
	}
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return sign + b.String()
}

// plural picks the ending for a count, so the window says "1 file" and not
// "1 files".
func plural(n int64, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// exactBytes spells a size out both ways: the compact form the listing uses,
// and the exact count beside it when the two differ.
func exactBytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d %s", n, plural(n, "byte", "bytes"))
	}
	return fmt.Sprintf("%s  (%s bytes)", humanSize(n), commas(n))
}
