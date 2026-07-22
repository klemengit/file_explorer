package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// holder is one process keeping a mountpoint busy, as reported to the user
// after a failed unmount.
type holder struct {
	pid  int
	name string // the process's command name, e.g. "nvim"
	what string // what it is holding: a path under the mountpoint
}

// udisksNoise matches the D-Bus error wrapper udisksctl prints in front of the
// real message, e.g. "GDBus.Error:org.freedesktop.UDisks2.Error.DeviceBusy: ".
var udisksNoise = regexp.MustCompile(`^(GDBus\.Error:)?[\w.]*Error\.\w+:\s*`)

// opNoise matches udisksctl's own "Error unmounting /dev/sda1: " preamble,
// which it repeats on both sides of the D-Bus wrapper.
var opNoise = regexp.MustCompile(`^Error \w+ing [^:]+:\s*`)

// cleanUdisksError strips the layers udisksctl wraps around a failure, so
//
//	Error unmounting /dev/sda1: GDBus.Error:org.freedesktop.UDisks2.Error.
//	DeviceBusy: Error unmounting /dev/sda1: target is busy
//
// becomes just "target is busy".
func cleanUdisksError(s string) string {
	s = strings.TrimSpace(s)
	for {
		next := strings.TrimSpace(opNoise.ReplaceAllString(udisksNoise.ReplaceAllString(s, ""), ""))
		if next == "" || next == s {
			return s
		}
		s = next
	}
}

// isBusyErr reports whether err is the kernel refusing to unmount because
// something still has the filesystem open.
func isBusyErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "busy")
}

// busyHolders finds the processes holding mp open by walking /proc directly,
// rather than shelling out to fuser or lsof — one less thing that has to be
// installed, and /proc tells us *what* each process is holding, not just that
// it is.
//
// procRoot is a parameter so tests can point it at a fake tree. We can only
// read the links of our own processes unless running as root, which is fine:
// the culprit is nearly always something the user started themselves (an nvim
// left open on a file, a shell sitting in the directory, a GUI viewer).
func busyHolders(procRoot, mp string) []holder {
	if mp == "" {
		return nil
	}
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return nil
	}
	self := os.Getpid()

	var out []holder
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid == self {
			continue // not a process directory, or it's us
		}
		dir := filepath.Join(procRoot, e.Name())
		what, ok := holdsMount(dir, mp)
		if !ok {
			continue
		}
		out = append(out, holder{pid: pid, name: procName(dir), what: what})
	}
	return out
}

// holdsMount reports whether the process at procDir has anything under mp: its
// working directory, its executable, or any open file. It returns the first
// such path, which is what makes the report actionable ("nvim … /notes.md"
// tells you which window to close).
func holdsMount(procDir, mp string) (string, bool) {
	for _, link := range []string{"cwd", "exe", "root"} {
		if p, err := os.Readlink(filepath.Join(procDir, link)); err == nil && underMount(p, mp) {
			return p, true
		}
	}
	fds, err := os.ReadDir(filepath.Join(procDir, "fd"))
	if err != nil {
		return "", false
	}
	for _, fd := range fds {
		if p, err := os.Readlink(filepath.Join(procDir, "fd", fd.Name())); err == nil && underMount(p, mp) {
			return p, true
		}
	}
	return "", false
}

// underMount reports whether p is the mountpoint itself or sits inside it.
// "/" is excluded: every process is under the root filesystem, so treating it
// as a mountpoint would name the entire process table.
func underMount(p, mp string) bool {
	if mp == "" || mp == "/" {
		return false
	}
	return p == mp || strings.HasPrefix(p, mp+"/")
}

// procName reads the process's command name, falling back to the first field
// of its command line when comm is unreadable.
func procName(procDir string) string {
	if b, err := os.ReadFile(filepath.Join(procDir, "comm")); err == nil {
		if s := strings.TrimSpace(string(b)); s != "" {
			return s
		}
	}
	if b, err := os.ReadFile(filepath.Join(procDir, "cmdline")); err == nil {
		if arg, _, _ := strings.Cut(string(b), "\x00"); arg != "" {
			return filepath.Base(arg)
		}
	}
	return "?"
}
