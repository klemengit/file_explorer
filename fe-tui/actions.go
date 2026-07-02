package main

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// clipEntry is the in-memory yank/cut clipboard (one item at a time).
type clipEntry struct {
	path string
	cut  bool // true = move on paste, false = copy
}

// copyPath copies src to dst, recursively for directories.
func copyPath(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return copyDir(src, dst, info)
	}
	return copyFile(src, dst, info)
}

func copyFile(src, dst string, info os.FileInfo) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func copyDir(src, dst string, info os.FileInfo) error {
	if err := os.MkdirAll(dst, info.Mode().Perm()); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		s := filepath.Join(src, e.Name())
		d := filepath.Join(dst, e.Name())
		if err := copyPath(s, d); err != nil {
			return err
		}
	}
	return nil
}

// movePath renames src to dst, falling back to copy+remove across filesystems.
func movePath(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := copyPath(src, dst); err != nil {
		return err
	}
	return os.RemoveAll(src)
}

// openDetached launches a command in the background, fully detached from the TUI
// (used for xdg-open and open-with).
func openDetached(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	return cmd.Start()
}

// runZip zips a file/dir, or unzips a .zip. Runs in the item's parent directory
// so archive paths stay relative. Returns a human-readable result.
func runZip(path string) (string, error) {
	parent := filepath.Dir(path)
	name := filepath.Base(path)
	if strings.HasSuffix(name, ".zip") {
		dest := strings.TrimSuffix(name, ".zip")
		cmd := exec.Command("unzip", "-q", name, "-d", dest)
		cmd.Dir = parent
		if err := cmd.Run(); err != nil {
			return "", err
		}
		return "Unzipped: " + dest + "/", nil
	}
	cmd := exec.Command("zip", "-r", name+".zip", name)
	cmd.Dir = parent
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return "Zipped: " + name + ".zip", nil
}
