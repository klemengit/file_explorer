package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestOverlayBoxSplicesLines(t *testing.T) {
	bg := strings.Join([]string{
		"aaaaaaaaaa",
		"bbbbbbbbbb",
		"cccccccccc",
		"dddddddddd",
	}, "\n")
	box := "XX\nYY"

	got := overlayBox(bg, box, 4, 1, 10)
	want := strings.Join([]string{
		"aaaaaaaaaa",
		"bbbbXXbbbb",
		"ccccYYcccc",
		"dddddddddd",
	}, "\n")
	if strip(got) != want {
		t.Errorf("overlay =\n%q\nwant\n%q", strip(got), want)
	}
}

// The overlay must not change the background's size, or the terminal scrolls.
func TestOverlayBoxKeepsBackgroundBounds(t *testing.T) {
	bg := strings.Join([]string{"aaaa", "bbbb", "cccc"}, "\n")

	got := overlayBox(bg, "XXXX\nYYYY\nZZZZ", 2, 2, 4) // runs off the bottom and right
	lines := strings.Split(strip(got), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3", len(lines))
	}
	for i, l := range lines {
		if ansi.StringWidth(l) != 4 {
			t.Errorf("line %d width %d, want 4: %q", i, ansi.StringWidth(l), l)
		}
	}
	if lines[2] != "ccXX" {
		t.Errorf("bottom line = %q, want the box clipped to the screen width", lines[2])
	}
}

// Splicing over a short background line pads it out rather than shifting the box left.
func TestOverlayBoxPadsShortLines(t *testing.T) {
	got := overlayBox("ab\n\ncd", "##\n##", 4, 0, 0)
	lines := strings.Split(strip(got), "\n")
	if lines[0] != "ab  ##" {
		t.Errorf("line 0 = %q, want %q", lines[0], "ab  ##")
	}
	if lines[1] != "    ##" {
		t.Errorf("line 1 = %q, want %q", lines[1], "    ##")
	}
}

// Styled backgrounds keep their colors, and the box's own styling is fenced off.
func TestOverlayBoxPreservesStyling(t *testing.T) {
	bg := dirStyle.Render("aaaaaaaaaa")
	box := errStyle.Render("XX")

	got := overlayBox(bg, box, 4, 0, 10)
	if strip(got) != "aaaaXXaaaa" {
		t.Errorf("stripped overlay = %q, want %q", strip(got), "aaaaXXaaaa")
	}
	if ansi.StringWidth(got) != 10 {
		t.Errorf("display width = %d, want 10", ansi.StringWidth(got))
	}
	if !strings.Contains(got, reset) {
		t.Error("expected reset sequences fencing the spliced box")
	}
}

func TestOverlayBoxEmptyForeground(t *testing.T) {
	if got := overlayBox("abc", "", 1, 1, 3); got != "abc" {
		t.Errorf("empty box changed the background: %q", got)
	}
}
