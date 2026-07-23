package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// chordModel is the goto test model sized like a real terminal, so the
// which-key window has somewhere to float.
func chordModel(t *testing.T) model {
	t.Helper()
	m, _, _ := gotoModel(t)
	m.width, m.height = 100, 24
	m.layout()
	return m
}

// send drives one message through the top-level Update and keeps the command,
// which is where the which-key tick lives.
func send(t *testing.T, m model, msg tea.Msg) (model, tea.Cmd) {
	t.Helper()
	tm, cmd := m.Update(msg)
	return tm.(model), cmd
}

// tick runs a command far enough to get the message it was going to deliver.
// tea.Tick sleeps first, so this blocks for the delay — which is short, and
// it is the only way to test that the real timer carries the real generation.
func tick(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("arming a chord should schedule its which-key window")
	}
	return cmd()
}

// Arming a chord asks for a window; the window only opens once the tick comes
// back, so a chord answered at speed never shows one.
func TestChordArmsThenOpensItsWindow(t *testing.T) {
	m := chordModel(t)

	m, cmd := send(t, m, keyRune('g'))
	if m.pending != "g" {
		t.Fatalf("pending = %q, want g", m.pending)
	}
	if m.whichKey {
		t.Error("the window must not be up before the delay has passed")
	}

	m, _ = send(t, m, tick(t, cmd))
	if !m.whichKey {
		t.Error("the tick should open the window")
	}
	if out := strip(m.View()); !strings.Contains(out, "goto") {
		t.Errorf("window is not titled after the chord:\n%s", out)
	}
}

// The window must never arrive late, over whatever you are doing by then.
func TestAnsweredChordDropsItsStaleTick(t *testing.T) {
	m := chordModel(t)

	m, cmd := send(t, m, keyRune('g'))
	stale := tick(t, cmd)

	m, _ = send(t, m, keyRune('d')) // chord answered before the tick lands
	if m.pending != "" {
		t.Fatalf("pending = %q, want the chord disarmed", m.pending)
	}
	m, _ = send(t, m, stale)
	if m.whichKey {
		t.Error("a tick from an answered chord opened a window")
	}
}

// Same for a chord that was abandoned rather than answered.
func TestCancelledChordDropsItsStaleTick(t *testing.T) {
	m := chordModel(t)

	m, cmd := send(t, m, keyRune('g'))
	stale := tick(t, cmd)

	m, _ = send(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	m, _ = send(t, m, stale)
	if m.whichKey {
		t.Error("a tick from a cancelled chord opened a window")
	}
}

// The case the generation counter exists for: an old tick arriving while a
// second chord happens to be pending. The window it would open belongs to a
// chord that is already over, so it must still be dropped — and the chord
// actually pending gets its own window from its own tick.
func TestStaleTickIsDroppedEvenWhileAnotherChordWaits(t *testing.T) {
	m := chordModel(t)

	m, first := send(t, m, keyRune('g'))
	stale := tick(t, first)
	m, _ = send(t, m, keyRune('g')) // gg: answered, chord over

	m, second := send(t, m, keyRune('g')) // armed again, a new chord
	m, _ = send(t, m, stale)
	if m.whichKey {
		t.Error("the first chord's tick opened a window for the second")
	}
	m, _ = send(t, m, tick(t, second))
	if !m.whichKey {
		t.Error("the pending chord should still get its own window")
	}
}

func TestWhichKeyWindowNamesTheKeys(t *testing.T) {
	m := chordModel(t)
	m.whichKey, m.pending = true, "g"

	out := strip(m.View())
	for _, want := range []string{"g — goto", "sub", "start dir", "other pane", "esc cancel"} {
		if !strings.Contains(out, want) {
			t.Errorf("which-key window missing %q:\n%s", want, out)
		}
	}
	// The panes stay visible around it — the chord acts on what you can see.
	if !strings.Contains(out, "a.txt") {
		t.Errorf("the panes vanished behind the which-key window:\n%s", out)
	}
}

func TestWhichKeyBoxIsRectangularAndFits(t *testing.T) {
	sizes := []struct{ w, h int }{{100, 24}, {60, 20}, {40, 12}, {32, 8}}
	for _, s := range sizes {
		m := chordModel(t)
		m.width, m.height = s.w, s.h
		m.layout()
		m.whichKey, m.pending = true, "g"

		box := m.whichKeyBox()
		assertRectangular(t, box)
		if w := lipgloss.Width(box); w > m.width {
			t.Errorf("%dx%d: window is %d wide", s.w, s.h, w)
		}
		view := m.View()
		if h := lipgloss.Height(view); h != m.height {
			t.Errorf("%dx%d: view is %d lines", s.w, s.h, h)
		}
		for i, l := range strings.Split(view, "\n") {
			if w := ansi.StringWidth(strip(l)); w > m.width {
				t.Errorf("%dx%d: line %d is %d cells", s.w, s.h, i, w)
			}
		}
	}
}

// A chord with more continuations than the screen can hold shows what fits and
// says how many it left out, rather than growing past the terminal.
func TestWhichKeyWindowSaysWhatItLeftOut(t *testing.T) {
	m := chordModel(t)
	m.width, m.height = 30, 8
	m.layout()
	m.gotos = nil
	for _, r := range "abcdefghijklmnopqrstuvwxyz" {
		m.gotos = append(m.gotos, gotoTarget{key: string(r), label: "target " + string(r)})
	}
	m.whichKey, m.pending = true, "g"

	box := m.whichKeyBox()
	assertRectangular(t, box)
	if out := strip(box); !strings.Contains(out, "more") {
		t.Errorf("window does not say how many chords it left out:\n%s", out)
	}
}

// The footer carries the hint until the window takes over the job.
func TestFooterYieldsToTheWhichKeyWindow(t *testing.T) {
	m := chordModel(t)

	m, cmd := send(t, m, keyRune('g'))
	if got := strip(m.footer()); !strings.Contains(got, "goto:") {
		t.Errorf("footer = %q, want the chord hint while the window is still coming", got)
	}
	m, _ = send(t, m, tick(t, cmd))
	if got := strip(m.footer()); strings.Contains(got, "goto:") {
		t.Errorf("footer = %q, want it to stop repeating the open window", got)
	}
}

// Nothing in the machinery knows about g: a chord registered under another key
// arms, explains itself and runs, with no other change to the code.
func TestChordsAreNotLimitedToG(t *testing.T) {
	var ran string
	restore := chordSet
	chordSet = append(append([]chord(nil), chordSet...), chord{
		key:   "Z",
		title: "test",
		entries: func(model) []chordEntry {
			return []chordEntry{{"a", "alpha"}, {"b", "bravo"}}
		},
		run: func(m model, key string) (tea.Model, tea.Cmd) {
			ran = key
			return m, nil
		},
	})
	t.Cleanup(func() { chordSet = restore })

	m := chordModel(t)
	m, cmd := send(t, m, keyRune('Z'))
	if m.pending != "Z" {
		t.Fatalf("pending = %q, want Z", m.pending)
	}
	if got := strip(m.footer()); !strings.Contains(got, "test:") || !strings.Contains(got, "a alpha") {
		t.Errorf("footer = %q, want the new chord's hint", got)
	}

	m, _ = send(t, m, tick(t, cmd))
	out := strip(m.View())
	for _, want := range []string{"Z — test", "alpha", "bravo"} {
		if !strings.Contains(out, want) {
			t.Errorf("which-key window missing %q:\n%s", want, out)
		}
	}

	m, _ = send(t, m, keyRune('b'))
	if ran != "b" {
		t.Errorf("the chord ran with %q, want b", ran)
	}
	if m.pending != "" || m.whichKey {
		t.Error("answering the chord should close it")
	}
}
