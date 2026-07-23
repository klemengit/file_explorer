package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// Nothing can be bound twice: a duplicate key would silently shadow whichever
// command came second in the registry.
func TestEveryKeyIsBoundOnce(t *testing.T) {
	seen := map[string]string{}
	for _, c := range commandSet {
		if len(c.keys) == 0 {
			t.Errorf("%q has no keys", c.desc)
		}
		if c.desc == "" || c.run == nil {
			t.Errorf("%v is missing a description or a body", c.keys)
		}
		for _, k := range c.keys {
			if prev, dup := seen[k]; dup {
				t.Errorf("key %q is bound to both %q and %q", k, prev, c.desc)
			}
			seen[k] = c.desc
		}
	}
}

// A chord prefix is checked before the commands, so a command bound to the same
// key could never run.
func TestNoCommandShadowsAChordPrefix(t *testing.T) {
	for _, ch := range chordSet {
		if c, ok := commandFor(ch.key); ok {
			t.Errorf("chord prefix %q is also bound to the command %q", ch.key, c.desc)
		}
	}
}

// The help is the registry rendered, so every command has to reach it — which
// is the duplication this replaced: the old hand-written list had lost pgup
// and pgdown.
func TestHelpListsEveryCommand(t *testing.T) {
	m, _ := selModel(t)
	var help []string
	for _, b := range m.helpBindings() {
		help = append(help, b.key+"\x00"+b.desc)
	}
	joined := strings.Join(help, "\n")
	for _, c := range commandSet {
		if !strings.Contains(joined, c.prettyKeys()+"\x00"+c.desc) {
			t.Errorf("help is missing %q (%s)", c.desc, c.prettyKeys())
		}
	}
	for _, want := range []string{"pgup", "pgdown"} {
		if !strings.Contains(joined, want) {
			t.Errorf("help is missing %q", want)
		}
	}
}

func TestPrettyKeysReadsLikeAKeyboard(t *testing.T) {
	cases := []struct {
		c    command
		want string
	}{
		{command{keys: []string{"j", "down"}}, "j / ↓"},
		{command{keys: []string{"l", "right", "enter"}}, "l / → / enter"},
		{command{keys: []string{" ", "space"}}, "space"}, // one key, two spellings
		{command{keys: []string{"ctrl+d"}}, "ctrl-d"},
		{command{keys: []string{"q"}, keyLabel: "q / ctrl-c"}, "q / ctrl-c"},
	}
	for _, c := range cases {
		if got := c.c.prettyKeys(); got != c.want {
			t.Errorf("prettyKeys(%v) = %q, want %q", c.c.keys, got, c.want)
		}
	}
}

// The keys still do what they did before there was a registry.
func TestRegistryDispatchStillRunsTheKeys(t *testing.T) {
	m, _ := selModel(t)
	m.width, m.height = 100, 24
	m.layout()

	if got := press(t, m, tea.KeyMsg{Type: tea.KeyTab}); got.active != 1 {
		t.Error("tab should switch panes")
	}
	if got := press(t, m, keyRune('.')); got.cur().showDots == m.cur().showDots {
		t.Error(". should toggle dotfiles")
	}
	if got := press(t, m, keyRune('?')); !got.showHelp {
		t.Error("? should open the help")
	}
	if got := press(t, m, keyRune('j'), keyRune('y')); got.clip == nil {
		t.Error("y should yank")
	}
}

// A command whose preconditions aren't met is not run — pasting an empty
// clipboard must not reach the paste code at all.
func TestUnavailableCommandsDoNotRun(t *testing.T) {
	m, _ := selModel(t)
	m.width, m.height = 100, 24
	m.layout()

	c, ok := commandFor("p")
	if !ok {
		t.Fatal("p is not bound")
	}
	if c.available(m) {
		t.Error("paste should be unavailable with an empty clipboard")
	}
	if got := press(t, m, keyRune('p')); got.status != "" {
		t.Errorf("p with nothing yanked reached the paste code: %q", got.status)
	}

	m.clip = &clipEntry{paths: []string{"/tmp/x"}}
	if !c.available(m) {
		t.Error("paste should be available once something is yanked")
	}
}
