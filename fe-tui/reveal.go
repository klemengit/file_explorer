package main

import (
	"bytes"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
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

// justDocOK reports whether just-doc is installed, looked up once like zoxide.
var justDocOK = sync.OnceValue(func() bool {
	_, err := exec.LookPath("just-doc")
	return err == nil
})

// justDocKind reports whether just-doc can open path: a PDF, Markdown or HTML
// file, judged by extension.
func justDocKind(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".pdf", ".md", ".markdown", ".html", ".htm":
		return true
	}
	return false
}

// justDocExitMsg reports a just-doc that has exited. The first one started
// keeps running as the server for every document, so a quick exit is either a
// hand-over to that server (err nil) or a failure, whose stderr says why.
type justDocExitMsg struct {
	name   string
	err    error
	stderr string
}

// startJustDoc launches just-doc on one file and returns a command that waits
// for it. It gets its own session so the server outlives the terminal fe runs
// in, and its stderr is kept so a failure — a Markdown file pandoc can't
// convert, say — is reported instead of vanishing.
func startJustDoc(path string) (tea.Cmd, error) {
	var stderr bytes.Buffer
	cmd := exec.Command("just-doc", path)
	cmd.Stderr = &stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return func() tea.Msg {
		err := cmd.Wait()
		return justDocExitMsg{name: filepath.Base(path), err: err, stderr: stderr.String()}
	}, nil
}

// justDocReason condenses just-doc's stderr to one status line: the first
// two non-blank lines, which for a pandoc failure are "could not convert" and
// the actual error.
func justDocReason(stderr string) string {
	var lines []string
	for _, l := range strings.Split(stderr, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
			if len(lines) == 2 {
				break
			}
		}
	}
	return strings.Join(lines, " ")
}

// openJustDoc opens the targets just-doc can read, one call each (it takes a
// single file), and says how many of the rest it passed over.
func (m *model) openJustDoc(targets []string) tea.Cmd {
	var docs []string
	for _, t := range targets {
		if justDocKind(t) {
			docs = append(docs, t)
		}
	}
	if len(docs) == 0 {
		m.setStatus(lvlWarn, "just-doc opens PDF, Markdown and HTML files only")
		return nil
	}
	var waits []tea.Cmd
	for _, d := range docs {
		wait, err := startJustDoc(d)
		if err != nil {
			m.setStatus(lvlErr, "just-doc %s: %v", filepath.Base(d), err)
			return tea.Batch(waits...)
		}
		waits = append(waits, wait)
	}
	if skipped := len(targets) - len(docs); skipped > 0 {
		m.setStatus(lvlWarn, "Opened %s in just-doc; skipped %d that aren't PDF, Markdown or HTML", describePaths(docs), skipped)
	} else {
		m.setStatus(lvlInfo, "Opened %s in just-doc", describePaths(docs))
	}
	return tea.Batch(waits...)
}
