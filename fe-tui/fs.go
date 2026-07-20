package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// entry is one item in a directory listing.
type entry struct {
	name    string
	isDir   bool
	isLink  bool
	size    int64
	modTime time.Time
}

type sortMode int

const (
	sortName     sortMode = iota // grouped dirs → links → files, by name
	sortTimeDesc                 // flat, newest first
	sortTimeAsc                  // flat, oldest first
)

// listDir reads dir and returns its entries. Sorting by name groups them
// dirs → symlinks → files (matching the original fe.sh), each group by name.
// Sorting by mtime is a flat, newest-first order across all entries (like
// `ls -t`), so the most recently modified item comes first regardless of type.
func listDir(dir string, sm sortMode, showDirs, showDots bool) ([]entry, error) {
	des, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var all []entry
	for _, de := range des {
		name := de.Name()
		if !showDots && strings.HasPrefix(name, ".") {
			continue
		}
		isLink := de.Type()&fs.ModeSymlink != 0
		isDir := de.IsDir()
		if isDir && !showDirs {
			continue
		}
		info, ierr := de.Info()
		var mt time.Time
		var sz int64
		if ierr == nil {
			mt = info.ModTime()
			sz = info.Size()
		}
		all = append(all, entry{name: name, isDir: isDir, isLink: isLink, size: sz, modTime: mt})
	}

	byName := func(i, j int) bool {
		return strings.ToLower(all[i].name) < strings.ToLower(all[j].name)
	}

	if sm == sortTimeDesc || sm == sortTimeAsc {
		// Flat mtime order; stable, with name as a tie-breaker.
		sort.SliceStable(all, func(i, j int) bool {
			if !all[i].modTime.Equal(all[j].modTime) {
				if sm == sortTimeDesc {
					return all[i].modTime.After(all[j].modTime)
				}
				return all[i].modTime.Before(all[j].modTime)
			}
			return byName(i, j)
		})
		return all, nil
	}

	// Group dirs → symlinks → files, each sorted by name.
	rank := func(e entry) int {
		switch {
		case e.isDir:
			return 0
		case e.isLink:
			return 1
		default:
			return 2
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		if r, s := rank(all[i]), rank(all[j]); r != s {
			return r < s
		}
		return byName(i, j)
	})
	return all, nil
}

// hasDotComponent reports whether any path component (relative to root) starts
// with a dot — used to skip hidden files/dirs in recursive walks.
func hasDotComponent(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if strings.HasPrefix(part, ".") && part != "." {
			return true
		}
	}
	return false
}

// deepFind returns every file and directory under dir (recursive, skipping
// dotted paths) as paths relative to dir. Mirrors the `f` deep-find action.
func deepFind(dir string) []string {
	var out []string
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if path == dir {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, rerr := filepath.Rel(dir, path)
		if rerr == nil {
			out = append(out, rel)
		}
		return nil
	})
	sort.Strings(out)
	return out
}
