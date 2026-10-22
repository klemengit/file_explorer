package main

import (
	"strings"
	"testing"
)

// ShowItems gets one comma-separated array argument, so a comma inside a path
// must be escaped or dbus-send would split it into two bogus URIs.
func TestRevealURIsEscapeCommas(t *testing.T) {
	got := revealURIs([]string{"/tmp/a,b.txt", "/tmp/with space"})
	want := "file:///tmp/a%2Cb.txt,file:///tmp/with%20space"
	if got != want {
		t.Errorf("revealURIs = %q, want %q", got, want)
	}
}

func TestRevealArgsCallShowItems(t *testing.T) {
	args := strings.Join(revealArgs([]string{"/tmp/x"}), " ")
	for _, part := range []string{
		"--dest=org.freedesktop.FileManager1",
		"org.freedesktop.FileManager1.ShowItems",
		"array:string:file:///tmp/x",
		"--print-reply",
	} {
		if !strings.Contains(args, part) {
			t.Errorf("revealArgs missing %q in %q", part, args)
		}
	}
}

func TestJustDocKind(t *testing.T) {
	for path, want := range map[string]bool{
		"a.pdf": true, "b.MD": true, "c.markdown": true, "d.html": true, "e.htm": true,
		"f.txt": false, "g": false, "pdf": false,
	} {
		if got := justDocKind(path); got != want {
			t.Errorf("justDocKind(%q) = %v, want %v", path, got, want)
		}
	}
}

// With nothing just-doc can read, J says so instead of launching anything.
func TestOpenJustDocSkipsOtherFiles(t *testing.T) {
	m := newModel(t.TempDir())
	m.openJustDoc([]string{"/tmp/notes.txt"})
	if !strings.Contains(m.status, "only") {
		t.Errorf("status = %q, want a note that only some files open", m.status)
	}
}

func TestRevealAndJustDocAreRegistered(t *testing.T) {
	if _, ok := commandFor("E"); !ok {
		t.Error("E is not bound")
	}
	c, ok := commandFor("J")
	if !ok {
		t.Fatal("J is not bound")
	}
	if c.unavailable == "" {
		t.Error("J should say why it is unavailable")
	}
}

// A pandoc failure arrives as a block of output; the status line gets the
// part that names the problem.
func TestJustDocReasonKeepsTheError(t *testing.T) {
	stderr := "pandoc could not convert README.md:\nerror: label `<x>` does not exist\n    ┌─ tmp.typ:293\n"
	want := "pandoc could not convert README.md: error: label `<x>` does not exist"
	if got := justDocReason(stderr); got != want {
		t.Errorf("justDocReason = %q, want %q", got, want)
	}
}
