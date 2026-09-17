package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// propsTree builds a small tree with known sizes:
//
//	root/big/one.bin      300 bytes
//	root/big/deep/two.bin 200 bytes
//	root/small/three.bin   50 bytes
//	root/loose.bin         10 bytes
//
// which is 560 bytes in 4 files across 3 subdirectories.
func propsTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel string, n int) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, make([]byte, n), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("big/one.bin", 300)
	write("big/deep/two.bin", 200)
	write("small/three.bin", 50)
	write("loose.bin", 10)
	return root
}

// awaitScan waits for a walk to finish, so a test reads a total and not a
// moment somewhere in the middle of one.
func awaitScan(t *testing.T, s *dirScan) scanState {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		st := s.state()
		if st.done {
			return st
		}
		if time.Now().After(deadline) {
			t.Fatal("scan did not finish")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestScanAddsUpTheWholeTree(t *testing.T) {
	st := awaitScan(t, startScan(propsTree(t)))

	if st.bytes != 560 {
		t.Errorf("bytes = %d, want 560", st.bytes)
	}
	if st.files != 4 {
		t.Errorf("files = %d, want 4", st.files)
	}
	if st.dirs != 3 {
		t.Errorf("dirs = %d, want 3 (big, big/deep, small)", st.dirs)
	}
	if st.skipped != 0 {
		t.Errorf("skipped = %d, want 0", st.skipped)
	}
}

// The largest list charges everything below a child to that child, so "big"
// carries the file in big/deep as well as its own.
func TestScanChargesBytesToTheTopLevelChild(t *testing.T) {
	st := awaitScan(t, startScan(propsTree(t)))

	want := []childSize{
		{name: "big", bytes: 500, isDir: true},
		{name: "small", bytes: 50, isDir: true},
		{name: "loose.bin", bytes: 10},
	}
	if len(st.largest) != len(want) {
		t.Fatalf("largest = %+v, want %d entries", st.largest, len(want))
	}
	for i, w := range want {
		if st.largest[i] != w {
			t.Errorf("largest[%d] = %+v, want %+v", i, st.largest[i], w)
		}
	}
}

// Dotfiles are part of what a directory takes up whether or not the pane shows
// them — a .git is usually the answer to "why is this so big".
func TestScanCountsDotfiles(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".hidden"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".hidden", "x.bin"), make([]byte, 128), 0o644); err != nil {
		t.Fatal(err)
	}

	st := awaitScan(t, startScan(root))
	if st.bytes != 128 || st.files != 1 {
		t.Errorf("got %d bytes in %d files, want 128 in 1", st.bytes, st.files)
	}
}

// A symlink pointing back up the tree would send a following walk round in
// circles. It is counted at its own size instead, and not followed.
func TestScanDoesNotFollowSymlinks(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.bin"), make([]byte, 100), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(root, filepath.Join(root, "loop")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	st := awaitScan(t, startScan(root))
	if st.files != 2 {
		t.Errorf("files = %d, want 2 (the file and the link itself)", st.files)
	}
	if st.bytes < 100 || st.bytes > 100+4096 {
		t.Errorf("bytes = %d, want the file plus the link's own small size", st.bytes)
	}
}

// An unreadable corner is counted and stepped over, so the window can say the
// total is a floor rather than abandoning the walk.
func TestScanCountsWhatItCannotRead(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads everything")
	}
	root := t.TempDir()
	locked := filepath.Join(root, "locked")
	if err := os.MkdirAll(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(locked, "x.bin"), make([]byte, 64), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o755) })

	st := awaitScan(t, startScan(root))
	if st.skipped == 0 {
		t.Error("skipped = 0, want the unreadable directory counted")
	}
	if st.bytes != 0 {
		t.Errorf("bytes = %d, want 0 — nothing under it could be read", st.bytes)
	}
}

func TestTopChild(t *testing.T) {
	cases := []struct{ root, path, want string }{
		{"/home/k/code", "/home/k/code/fe", "fe"},
		{"/home/k/code", "/home/k/code/fe/main.go", "fe"},
		{"/home/k/code/", "/home/k/code/fe/main.go", "fe"},
		{"/", "/etc", "etc"},
		{"/", "/etc/hosts", "etc"},
	}
	for _, c := range cases {
		if got := topChild(c.root, c.path); got != c.want {
			t.Errorf("topChild(%q, %q) = %q, want %q", c.root, c.path, got, c.want)
		}
	}
}

func TestCommas(t *testing.T) {
	cases := map[int64]string{0: "0", 7: "7", 999: "999", 1000: "1,000",
		12481: "12,481", 5043201003: "5,043,201,003", -1234: "-1,234"}
	for n, want := range cases {
		if got := commas(n); got != want {
			t.Errorf("commas(%d) = %q, want %q", n, got, want)
		}
	}
}

// Under a kilobyte the compact form is the exact form, so it is said once.
func TestExactBytes(t *testing.T) {
	if got, want := exactBytes(1), "1 byte"; got != want {
		t.Errorf("exactBytes(1) = %q, want %q", got, want)
	}
	if got, want := exactBytes(512), "512 bytes"; got != want {
		t.Errorf("exactBytes(512) = %q, want %q", got, want)
	}
	if got := exactBytes(2048); !strings.Contains(got, "2.0K") || !strings.Contains(got, "2,048 bytes") {
		t.Errorf("exactBytes(2048) = %q, want both forms", got)
	}
}

// s opens the window on the entry under the cursor, and esc closes it and calls
// off the walk it started.
func TestPropsOpensOnTheEntryUnderTheCursor(t *testing.T) {
	root := propsTree(t)
	m := newModel(root)
	m.width, m.height = 100, 30
	m.layout()
	m.cur().cursor = 1 // "big", the first entry after ".."

	m = press(t, m, keyRune('s'))
	if m.mode != modeProps {
		t.Fatalf("mode = %v, want modeProps", m.mode)
	}
	if want := filepath.Join(root, "big"); m.props.path != want {
		t.Errorf("props.path = %q, want %q", m.props.path, want)
	}
	if m.props.scan == nil {
		t.Fatal("a directory should be walked")
	}
	scan := m.props.scan

	tm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = tm.(model)
	if m.mode != modeBrowse || m.props != nil {
		t.Errorf("esc left mode = %v, props = %+v", m.mode, m.props)
	}
	if !scan.stopped() {
		t.Error("closing the window should call off the walk")
	}
}

// Sitting on ".." and asking for properties is how you get the size of the
// directory you are in.
func TestPropsOnTheParentRowDescribesTheCurrentDirectory(t *testing.T) {
	root := propsTree(t)
	m := newModel(root)
	m.width, m.height = 100, 30
	m.layout()
	m.cur().cursor = 0 // ".."

	m = press(t, m, keyRune('s'))
	if m.props == nil || m.props.path != root {
		t.Fatalf("props.path = %+v, want %q", m.props, root)
	}
	awaitScan(t, m.props.scan)
	if st := m.props.scan.state(); st.bytes != 560 {
		t.Errorf("bytes = %d, want the whole tree's 560", st.bytes)
	}
}

// A file needs no walk: its size is the one stat already answered.
func TestPropsOnAFileNeedsNoWalk(t *testing.T) {
	root := propsTree(t)
	m := newModel(root)
	m.width, m.height = 100, 30
	m.layout()
	for i, r := range m.cur().rows {
		if r.name == "loose.bin" {
			m.cur().cursor = i
		}
	}

	m = press(t, m, keyRune('s'))
	if m.props == nil || m.props.scan != nil {
		t.Fatalf("props = %+v, want a file with no scan", m.props)
	}
	if m.props.size != 10 {
		t.Errorf("size = %d, want 10", m.props.size)
	}
	if body := strip(m.propsBox()); !strings.Contains(body, "10 bytes") {
		t.Errorf("window does not give the size:\n%s", body)
	}
}

// The finished window says the total, what is in it, and where the space went.
func TestPropsBoxReportsTheFinishedWalk(t *testing.T) {
	root := propsTree(t)
	m := newModel(root)
	m.width, m.height = 100, 30
	m.layout()
	m = press(t, m, keyRune('s'))
	awaitScan(t, m.props.scan)

	body := strip(m.propsBox())
	for _, want := range []string{"properties", "directory", "560 bytes",
		"4 files", "3 directories", "largest inside", "big/", "esc close"} {
		if !strings.Contains(body, want) {
			t.Errorf("window is missing %q:\n%s", want, body)
		}
	}
}

// While it is still counting, the window says so rather than showing a total
// that is not one yet.
func TestPropsBoxSaysItIsStillCounting(t *testing.T) {
	m := newModel(propsTree(t))
	m.width, m.height = 100, 30
	m.layout()
	m.props = &props{path: "/x", name: "x", isDir: true, scan: &dirScan{
		children:    map[string]int64{},
		dirChildren: map[string]bool{},
		cancel:      make(chan struct{}),
	}}
	m.mode = modeProps

	body := strip(m.propsBox())
	if !strings.Contains(body, "so far") || !strings.Contains(body, "counting") {
		t.Errorf("window does not say it is still counting:\n%s", body)
	}
}

// A short terminal keeps the facts and gives up the largest-inside list, rather
// than growing a window taller than the screen.
func TestPropsBoxGivesUpTheListOnAShortTerminal(t *testing.T) {
	m := newModel(propsTree(t))
	m.width, m.height = 100, 30
	m.layout()
	m = press(t, m, keyRune('s'))
	awaitScan(t, m.props.scan)

	if body := strip(m.propsBox()); !strings.Contains(body, "largest inside") {
		t.Fatalf("the tall window should list the largest entries:\n%s", body)
	}

	m.height = 14
	body := strip(m.propsBox())
	if strings.Contains(body, "largest inside") {
		t.Errorf("a 14-row terminal should not get the list:\n%s", body)
	}
	if !strings.Contains(body, "560 bytes") {
		t.Errorf("the size is the point and must survive:\n%s", body)
	}
	if h := strings.Count(body, "\n") + 1; h > 14 {
		t.Errorf("window is %d rows tall on a 14-row terminal", h)
	}
}
