package main

import (
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
)

// revealURIs turns paths into the file:// URIs ShowItems takes. dbus-send
// splits an array argument on commas and url.URL leaves them alone, so they are
// escaped by hand; %2C is still a valid file URI for the same path.
func revealURIs(paths []string) string {
	uris := make([]string, len(paths))
	for i, p := range paths {
		u := url.URL{Scheme: "file", Path: p}
		uris[i] = strings.ReplaceAll(u.String(), ",", "%2C")
	}
	return strings.Join(uris, ",")
}

// revealArgs is the dbus-send call that asks the desktop's file manager to
// open the folder holding paths with them selected. org.freedesktop.FileManager1
// is the shared interface for this — Nautilus, Dolphin, Nemo, Thunar and
// Caja all answer it. --print-reply makes dbus-send wait for the answer, so a
// desktop with no such service fails here instead of silently doing nothing.
func revealArgs(paths []string) []string {
	return []string{
		"--session", "--print-reply", "--reply-timeout=3000",
		"--dest=org.freedesktop.FileManager1",
		"/org/freedesktop/FileManager1",
		"org.freedesktop.FileManager1.ShowItems",
		"array:string:" + revealURIs(paths),
		"string:",
	}
}

// reveal shows targets in the system file manager, selected in their folder.
// With nothing to point at it opens the current directory, which is all E did
// before. If the file manager doesn't speak FileManager1, it falls back to
// opening the folder of the first target with xdg-open — the right place,
// just without the highlight.
func (m *model) reveal(targets []string) {
	if len(targets) == 0 {
		if err := openDetached("xdg-open", m.cur().dir); err != nil {
			m.setStatus(lvlErr, "xdg-open: %v", err)
		}
		return
	}
	if err := exec.Command("dbus-send", revealArgs(targets)...).Run(); err == nil {
		m.setStatus(lvlInfo, "Revealed %s", describePaths(targets))
		return
	}
	if err := openDetached("xdg-open", filepath.Dir(targets[0])); err != nil {
		m.setStatus(lvlErr, "xdg-open: %v", err)
		return
	}
	m.setStatus(lvlWarn, "Opened the folder; this file manager can't highlight %s", describePaths(targets))
}
