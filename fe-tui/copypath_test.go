package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// Every copy choice carries a key, and no two choices share one — otherwise
// `c d` would be ambiguous.
func TestCopyChoiceKeysAreDistinct(t *testing.T) {
	seen := map[string]string{}
	for _, ch := range copyChoicesFor([]string{"/home/u/dir/file.txt"}) {
		if ch.key == "" {
			t.Errorf("choice %q has no key", ch.label)
		}
		if prev, dup := seen[ch.key]; dup {
			t.Errorf("key %q picks both %q and %q", ch.key, prev, ch.label)
		}
		seen[ch.key] = ch.label
	}
}

func TestCopyKeyPicksTheChoice(t *testing.T) {
	m := model{copyItems: copyChoicesFor([]string{"/home/u/dir/file.txt"}), pickerKind: pickCopy}
	idx, ok := m.pickerKeyFor("d")
	if !ok {
		t.Fatal("d should pick a copy choice")
	}
	if got := m.copyItems[idx]; got.label != "directory" || got.value != "/home/u/dir" {
		t.Errorf("d picked %+v, want the directory", got)
	}
	if _, ok := m.pickerKeyFor("z"); ok {
		t.Error("z picks nothing, but pickerKeyFor said it does")
	}
}

// A keyed picker has no filter, so an unknown key does nothing at all rather
// than narrowing the list out from under you.
func TestKeyedPickerIgnoresUnknownKeys(t *testing.T) {
	tm, _ := model{width: 100, height: 28}.openCopyMenu([]string{"/home/u/dir/file.txt"})
	m := tm.(model)
	rows := len(m.pickerRows)

	tm, _ = m.updatePicker(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'z'}})
	m = tm.(model)
	if m.mode != modePicker {
		t.Errorf("unknown key left mode %v, want modePicker", m.mode)
	}
	if m.ti.Value() != "" {
		t.Errorf("unknown key typed %q into a menu with no filter", m.ti.Value())
	}
	if len(m.pickerRows) != rows {
		t.Errorf("row count changed from %d to %d", rows, len(m.pickerRows))
	}
}

// The key is printed beside the label, and the hint points at it instead of at
// a filter that isn't there.
func TestCopyMenuShowsItsKeys(t *testing.T) {
	tm, _ := model{width: 100, height: 28}.openCopyMenu([]string{"/home/u/dir/file.txt"})
	m := tm.(model)
	out := strip(m.pickerBox())
	for _, want := range []string{"d  directory", "a  absolute path", "press the key shown"} {
		if !strings.Contains(out, want) {
			t.Errorf("copy menu missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "type to filter") {
		t.Errorf("copy menu still offers a filter:\n%s", out)
	}
	assertRectangular(t, m.pickerBox())
}
