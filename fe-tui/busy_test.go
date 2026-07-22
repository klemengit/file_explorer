package main

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestCleanUdisksError(t *testing.T) {
	cases := []struct{ in, want string }{
		{
			// The message that started this: udisksctl repeats its own
			// preamble on both sides of the D-Bus wrapper.
			"Error unmounting /dev/sda1: GDBus.Error:org.freedesktop.UDisks2.Error.DeviceBusy: Error unmounting /dev/sda1: target is busy",
			"target is busy",
		},
		{
			"GDBus.Error:org.freedesktop.UDisks2.Error.NotAuthorized: Not authorized to perform operation",
			"Not authorized to perform operation",
		},
		{"umount: /run/media/u/USB: target is busy.", "umount: /run/media/u/USB: target is busy."},
		{"plain failure", "plain failure"},
		{"", ""},
	}
	for _, c := range cases {
		if got := cleanUdisksError(c.in); got != c.want {
			t.Errorf("cleanUdisksError(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIsBusyErr(t *testing.T) {
	if !isBusyErr(errors.New("target is busy")) {
		t.Error("want busy for 'target is busy'")
	}
	if !isBusyErr(errors.New("Device Busy")) {
		t.Error("want busy regardless of case")
	}
	if isBusyErr(errors.New("Not authorized")) {
		t.Error("want not-busy for an authorization failure")
	}
	if isBusyErr(nil) {
		t.Error("want not-busy for nil")
	}
}

func TestUnderMount(t *testing.T) {
	mp := "/run/media/u/USB"
	for _, p := range []string{mp, mp + "/notes.md", mp + "/a/b"} {
		if !underMount(p, mp) {
			t.Errorf("%q should be under %q", p, mp)
		}
	}
	for _, p := range []string{"/run/media/u/USB2", "/home/u", "/run/media/u"} {
		if underMount(p, mp) {
			t.Errorf("%q should not be under %q", p, mp)
		}
	}
	// "/" would match every process on the machine, so it never counts.
	if underMount("/home/u/x", "/") {
		t.Error("root filesystem must not be treated as a mountpoint")
	}
	if underMount("/anything", "") {
		t.Error("empty mountpoint must match nothing")
	}
}

// fakeProc builds a directory that looks enough like /proc for busyHolders:
// numbered process directories holding a comm file and symlinks for cwd and
// open file descriptors.
func fakeProc(t *testing.T, procs map[int]struct {
	comm string
	cwd  string
	fds  []string
}) string {
	t.Helper()
	root := t.TempDir()
	for pid, p := range procs {
		dir := filepath.Join(root, strconv.Itoa(pid))
		if err := os.MkdirAll(filepath.Join(dir, "fd"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "comm"), []byte(p.comm+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if p.cwd != "" {
			// Dangling symlinks are fine: we only ever read the link target.
			if err := os.Symlink(p.cwd, filepath.Join(dir, "cwd")); err != nil {
				t.Fatal(err)
			}
		}
		for i, target := range p.fds {
			if err := os.Symlink(target, filepath.Join(dir, "fd", strconv.Itoa(i))); err != nil {
				t.Fatal(err)
			}
		}
	}
	// A non-numeric entry, as /proc really has ("self", "meminfo", …).
	if err := os.MkdirAll(filepath.Join(root, "meminfo"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestBusyHoldersFindsCwdAndOpenFiles(t *testing.T) {
	mp := "/run/media/u/USB"
	root := fakeProc(t, map[int]struct {
		comm string
		cwd  string
		fds  []string
	}{
		101: {comm: "bash", cwd: mp},                                     // a shell sitting in the drive
		102: {comm: "nvim", cwd: "/home/u", fds: []string{mp + "/n.md"}}, // an editor with a file open
		103: {comm: "firefox", cwd: "/home/u", fds: []string{"/home/u/x"}},
	})

	hs := busyHolders(root, mp)
	if len(hs) != 2 {
		t.Fatalf("got %d holders, want 2: %+v", len(hs), hs)
	}
	byPID := map[int]holder{}
	for _, h := range hs {
		byPID[h.pid] = h
	}
	if h := byPID[101]; h.name != "bash" || h.what != mp {
		t.Errorf("pid 101 = %+v, want bash holding the mountpoint", h)
	}
	if h := byPID[102]; h.name != "nvim" || h.what != mp+"/n.md" {
		t.Errorf("pid 102 = %+v, want nvim holding n.md", h)
	}
	if _, ok := byPID[103]; ok {
		t.Error("firefox touches nothing on the drive and must not be listed")
	}
}

func TestBusyHoldersIgnoresSelfAndBadRoot(t *testing.T) {
	mp := "/run/media/u/USB"
	root := fakeProc(t, map[int]struct {
		comm string
		cwd  string
		fds  []string
	}{
		os.Getpid(): {comm: "fe", cwd: mp},
	})
	if hs := busyHolders(root, mp); len(hs) != 0 {
		t.Errorf("fe must never report itself as a holder, got %+v", hs)
	}
	if hs := busyHolders(filepath.Join(root, "nope"), mp); hs != nil {
		t.Errorf("unreadable proc root should yield no holders, got %+v", hs)
	}
	if hs := busyHolders(root, ""); hs != nil {
		t.Errorf("empty mountpoint should yield no holders, got %+v", hs)
	}
}

func TestDescribeHolders(t *testing.T) {
	if got := describeHolders(nil); !strings.Contains(got, "something") {
		t.Errorf("no-holders text should stay vague, got %q", got)
	}
	got := describeHolders([]holder{{pid: 42, name: "nvim"}})
	if !strings.Contains(got, "nvim") || !strings.Contains(got, "42") {
		t.Errorf("single holder should be named, got %q", got)
	}
	got = describeHolders([]holder{{pid: 1, name: "a"}, {pid: 2, name: "b"}})
	if !strings.Contains(got, "2 processes") {
		t.Errorf("multiple holders should be counted, got %q", got)
	}
}
