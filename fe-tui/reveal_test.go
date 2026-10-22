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

func TestRevealIsRegistered(t *testing.T) {
	if _, ok := commandFor("E"); !ok {
		t.Error("E is not bound")
	}
}
