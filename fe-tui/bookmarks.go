package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// bookmarksPath is ${XDG_DATA_HOME:-~/.local/share}/fe/bookmarks — the same
// location the original fe.sh uses, so bookmarks carry over.
func bookmarksPath() string {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "fe", "bookmarks")
}

func loadBookmarks() []string {
	f, err := os.Open(bookmarksPath())
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// addBookmark stores dir; returns false if it was already bookmarked.
func addBookmark(dir string) (bool, error) {
	for _, b := range loadBookmarks() {
		if b == dir {
			return false, nil
		}
	}
	p := bookmarksPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return false, err
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return false, err
	}
	defer f.Close()
	_, err = f.WriteString(dir + "\n")
	return true, err
}

func removeBookmark(dir string) error {
	marks := loadBookmarks()
	var kept []string
	for _, b := range marks {
		if b != dir {
			kept = append(kept, b)
		}
	}
	return os.WriteFile(bookmarksPath(), []byte(strings.Join(kept, "\n")+"\n"), 0o644)
}
