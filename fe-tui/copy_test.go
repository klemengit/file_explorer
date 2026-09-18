package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTree writes a small directory tree for copy tests: file.txt (one byte),
// sub/ nested inside it.
func writeTree(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "deep.bin"), []byte("yyyy"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestCopyProgressRunCopiesTree checks a directory copy lands every file and
// reports a byte count and per-entry done count.
func TestCopyProgressRunCopiesTree(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	writeTree(t, src)

	cp := &copyProgress{cancel: make(chan struct{})}
	if err := cp.run([]string{src}, dst, false); err != nil {
		t.Fatalf("run: %v", err)
	}
	landed := filepath.Join(dst, filepath.Base(src))
	for _, want := range []string{"file.txt", filepath.Join("sub", "deep.bin")} {
		if _, err := os.Lstat(filepath.Join(landed, want)); err != nil {
			t.Errorf("missing %s: %v", want, err)
		}
	}
	st := cp.state()
	if st.done != true {
		t.Errorf("done = %v, want true", st.done)
	}
	if st.doneFiles != 1 {
		t.Errorf("doneFiles = %d, want 1 (the top-level entry)", st.doneFiles)
	}
	// One byte plus four bytes across the tree.
	if st.bytesDone != 5 {
		t.Errorf("bytesDone = %d, want 5", st.bytesDone)
	}
}

// TestCopyProgressRunMoves checks a same-filesystem move is a rename (no bytes
// counted, but the entry is done and the source is gone).
func TestCopyProgressRunMoves(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	writeTree(t, src)
	cp := &copyProgress{cancel: make(chan struct{})}
	if err := cp.run([]string{filepath.Join(src, "file.txt")}, dst, true); err != nil {
		t.Fatalf("run: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dst, "file.txt")); err != nil {
		t.Errorf("missing dest file: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(src, "file.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("source should be gone, got %v", err)
	}
	st := cp.state()
	if st.doneFiles != 1 || st.bytesDone != 0 {
		t.Errorf("doneFiles=%d bytesDone=%d, want 1 and 0 for a rename", st.doneFiles, st.bytesDone)
	}
}

// TestCopyProgressCancel checks a cancelled run stops, marks itself done and
// returns the cancel sentinel rather than a real error.
func TestCopyProgressCancel(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	writeTree(t, src)
	cp := &copyProgress{cancel: make(chan struct{})}
	cp.stop() // cancelled before the run even starts
	if err := cp.run([]string{src}, dst, false); !errors.Is(err, errCopyCancelled) {
		t.Fatalf("run = %v, want errCopyCancelled", err)
	}
	if !cp.state().done {
		t.Errorf("done = false, want true after a cancelled run")
	}
}

// TestCopyProgressRunFailure checks a failed entry stops the run, records the
// error and marks done.
func TestCopyProgressRunFailure(t *testing.T) {
	dst := t.TempDir()
	cp := &copyProgress{cancel: make(chan struct{})}
	err := cp.run([]string{filepath.Join(dst, "nope")}, dst, false)
	if err == nil {
		t.Fatal("run on a missing source should fail")
	}
	st := cp.state()
	if !st.done {
		t.Errorf("done = false, want true after a failed run")
	}
	if st.doneFiles != 0 {
		t.Errorf("doneFiles = %d, want 0 (nothing completed)", st.doneFiles)
	}
}

// TestCopyNeedsProgress checks the background-run thresholds: several entries
// or a big enough total go async, a single small file stays synchronous.
func TestCopyNeedsProgress(t *testing.T) {
	dir := t.TempDir()
	small := filepath.Join(dir, "small")
	big := filepath.Join(dir, "big")
	one := filepath.Join(dir, "one")
	if err := os.WriteFile(small, make([]byte, 4), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(big, make([]byte, bigCopyThreshold), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(one, make([]byte, 4), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, _, big2 := copyNeedsProgress([]string{small}); big2 {
		t.Errorf("one small file should stay synchronous")
	}
	if _, _, big2 := copyNeedsProgress([]string{small, one}); !big2 {
		t.Errorf("two files should run in the background")
	}
	if _, _, big2 := copyNeedsProgress([]string{big}); !big2 {
		t.Errorf("one file over the byte threshold should run in the background")
	}
}

// TestCopyProgressLine checks the status line names the verb, the current
// entry, a byte percentage and the file counter.
func TestCopyProgressLine(t *testing.T) {
	st := copyState{current: "notes.txt", totalFiles: 2, doneFiles: 1, bytesDone: 25, totalBytes: 100}
	if got := copyProgressLine("copy", st); !strings.Contains(got, "Copying notes.txt — 25B / 100B (25%) · 1/2") {
		t.Errorf("copyProgressLine = %q", got)
	}
	if got := copyProgressLine("move", st); !strings.Contains(got, "Moving") {
		t.Errorf("move line should say Moving, got %q", got)
	}
	// No byte total: fall back to the file counter only.
	st2 := copyState{totalFiles: 5, doneFiles: 2, current: "x"}
	if got := copyProgressLine("copy", st2); !strings.Contains(got, "2/5") || strings.Contains(got, "%") {
		t.Errorf("no-total line = %q", got)
	}
}
