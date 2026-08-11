package main

import (
	"os"
	"os/exec"
	"strings"
	"sync"
)

// Z jumps to a directory zoxide already knows about.
//
// fe only ever reads the database. It does not record the directories you
// browse to, so `Z` reaches exactly the places your shell taught zoxide about
// and nothing fe added behind your back — with one exception you asked for by
// pressing the key: ctrl-d forgets the highlighted entry.

// zoxideOK reports whether zoxide is installed. Looked up once, because the
// answer cannot change while fe runs and the command palette asks it on every
// redraw to decide whether to grey the entry out.
var zoxideOK = sync.OnceValue(func() bool {
	_, err := exec.LookPath("zoxide")
	return err == nil
})

// zoxideList returns the directories zoxide knows, best-frecency first, with
// the ones that no longer exist left out.
//
// Bookmarks report a dead entry when you try to jump to it, because that list
// is curated by hand and a stale one is a surprise worth mentioning. A zoxide
// database collects dead directories as a matter of course — every temporary
// checkout you ever cd'd into — so listing them would be noise.
func zoxideList() []string {
	out, err := exec.Command("zoxide", "query", "-l").Output()
	if err != nil {
		return nil
	}
	return existingDirs(parseZoxideList(string(out)))
}

// parseZoxideList splits `zoxide query -l` output into paths, keeping the
// frecency order it comes in and skipping blank lines.
func parseZoxideList(out string) []string {
	var paths []string
	for _, line := range strings.Split(out, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			paths = append(paths, line)
		}
	}
	return paths
}

// existingDirs keeps only the paths that are still directories.
func existingDirs(paths []string) []string {
	kept := make([]string, 0, len(paths))
	for _, p := range paths {
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			kept = append(kept, p)
		}
	}
	return kept
}

// zoxideForget drops a path from the database. The directory itself is
// untouched — this is the picker's ctrl-d, mirroring the one that deletes a
// bookmark.
func zoxideForget(path string) error {
	return exec.Command("zoxide", "remove", path).Run()
}

// zoxideMatch reports whether path matches the typed query, using zoxide's own
// rule rather than fe's plain subsequence match: the terms must appear in the
// path in order, and the last term has to land in the final path component.
//
// That last clause is the whole difference. Typing "git" with fe's fuzzy match
// would offer every directory that merely lives somewhere under a "git_repos";
// zoxide only offers the ones actually called something with "git" in it, which
// is the behaviour `z git` already has in your shell.
func zoxideMatch(query, path string) bool {
	terms := strings.Fields(strings.ToLower(query))
	if len(terms) == 0 {
		return true
	}
	lower := strings.ToLower(path)

	// Every term, in order, each one after the last.
	at := 0
	for _, t := range terms {
		i := strings.Index(lower[at:], t)
		if i < 0 {
			return false
		}
		at += i + len(t)
	}

	// And the final term inside the final component.
	base := lower[strings.LastIndexByte(lower, '/')+1:]
	return strings.Contains(base, terms[len(terms)-1])
}
