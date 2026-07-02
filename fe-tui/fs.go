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
	sortName sortMode = iota
	sortTime
)

// listDir reads dir and returns its entries, grouped dirs → symlinks → files
// (matching the original fe.sh), each group sorted by name or mtime.
func listDir(dir string, sm sortMode, showDirs, showDots bool) ([]entry, error) {
	des, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var dirs, links, files []entry
	for _, de := range des {
		name := de.Name()
		if !showDots && strings.HasPrefix(name, ".") {
			continue
		}
		info, ierr := de.Info()
		var mt time.Time
		var sz int64
		if ierr == nil {
			mt = info.ModTime()
			sz = info.Size()
		}
		e := entry{name: name, modTime: mt, size: sz}
		switch {
		case de.Type()&fs.ModeSymlink != 0:
			e.isLink = true
			links = append(links, e)
		case de.IsDir():
			e.isDir = true
			dirs = append(dirs, e)
		default:
			files = append(files, e)
		}
	}

	sortGroup := func(g []entry) {
		sort.SliceStable(g, func(i, j int) bool {
			if sm == sortTime {
				return g[i].modTime.After(g[j].modTime)
			}
			return strings.ToLower(g[i].name) < strings.ToLower(g[j].name)
		})
	}
	sortGroup(dirs)
	sortGroup(links)
	sortGroup(files)

	out := make([]entry, 0, len(dirs)+len(links)+len(files))
	if showDirs {
		out = append(out, dirs...)
	}
	out = append(out, links...)
	out = append(out, files...)
	return out, nil
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

// recentFiles returns the n most-recently-modified files under dir (recursive,
// skipping dotfiles), as paths relative to dir. Mirrors the `n` action.
func recentFiles(dir string, n int) []string {
	type fm struct {
		rel string
		mt  time.Time
	}
	var all []fm
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") && path != dir {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			return nil
		}
		rel, rerr := filepath.Rel(dir, path)
		if rerr == nil {
			all = append(all, fm{rel: rel, mt: info.ModTime()})
		}
		return nil
	})
	sort.Slice(all, func(i, j int) bool { return all[i].mt.After(all[j].mt) })
	if len(all) > n {
		all = all[:n]
	}
	out := make([]string, len(all))
	for i, f := range all {
		out[i] = f.rel
	}
	return out
}
