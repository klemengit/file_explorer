package main

import (
	"regexp"
	"strings"
	"testing"
	"time"
)

var ansi = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func strip(s string) string { return ansi.ReplaceAllString(s, "") }

func TestHumanSize(t *testing.T) {
	cases := map[int64]string{0: "0B", 512: "512B", 1500: "1.5K", 4200000: "4.0M"}
	for in, want := range cases {
		if got := humanSize(in); got != want {
			t.Errorf("humanSize(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestRenderRowColumns(t *testing.T) {
	m := model{width: 90}
	mt := time.Date(2026, 1, 2, 15, 4, 0, 0, time.UTC)

	file := row{label: "alpha.txt", name: "alpha.txt", size: 1500, modTime: mt}
	out := strip(m.renderRow(file, false))
	if !strings.Contains(out, "1.5K") {
		t.Errorf("file row missing size: %q", out)
	}
	if !strings.Contains(out, "2026-01-02 15:04") {
		t.Errorf("file row missing date: %q", out)
	}

	dir := row{label: "sub/", name: "sub", isDir: true, modTime: mt}
	out = strip(m.renderRow(dir, false))
	if !strings.Contains(out, "-") {
		t.Errorf("dir row should show '-' for size: %q", out)
	}

	parent := row{label: "..", isParent: true}
	out = strip(m.renderRow(parent, true))
	if strings.Contains(out, "2026") {
		t.Errorf("parent row should have no date: %q", out)
	}

	// selected file row still carries both columns
	out = strip(m.renderRow(file, true))
	if !strings.Contains(out, "1.5K") || !strings.Contains(out, "2026-01-02 15:04") {
		t.Errorf("selected file row missing columns: %q", out)
	}
}
