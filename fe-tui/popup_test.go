package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// boxLines returns a rendered popup's lines with styling stripped.
func boxLines(box string) []string { return strings.Split(strip(box), "\n") }

// assertRectangular checks every line of a box is the same display width, which
// is what keeps the right border straight.
func assertRectangular(t *testing.T, box string) {
	t.Helper()
	lines := boxLines(box)
	w := ansi.StringWidth(lines[0])
	for i, l := range lines {
		if got := ansi.StringWidth(l); got != w {
			t.Fatalf("line %d is %d wide, want %d:\n%s", i, got, w, strip(box))
		}
	}
}

func TestPopupBoxIsRectangular(t *testing.T) {
	body := []string{"short", "a much longer body line than the others"}
	box := popupBox("title", "a hint", body, 40)
	assertRectangular(t, box)

	out := strip(box)
	for _, want := range []string{"title", "a hint", "short"} {
		if !strings.Contains(out, want) {
			t.Errorf("box missing %q:\n%s", want, out)
		}
	}
	if w := lipgloss.Width(box); w != 42 { // inner + two border columns
		t.Errorf("box width = %d, want 42", w)
	}
}

func TestPadLineMeasuresDisplayWidth(t *testing.T) {
	if got := padLine("ab", 5); got != "ab   " {
		t.Errorf("padLine(%q, 5) = %q", "ab", got)
	}
	// Styling must not count towards the width.
	styled := padLine(dirStyle.Render("ab"), 5)
	if w := ansi.StringWidth(styled); w != 5 {
		t.Errorf("styled line width = %d, want 5", w)
	}
	// Too long: trimmed to fit rather than pushing the border out.
	if w := ansi.StringWidth(padLine("abcdefgh", 4)); w != 4 {
		t.Errorf("over-long line width = %d, want 4", w)
	}
}

func TestPopupInnerClamps(t *testing.T) {
	m := model{width: 40}
	if got := m.popupInner(100); got != 34 { // width - popupPad
		t.Errorf("popupInner(100) = %d, want 34", got)
	}
	if got := m.popupInner(20); got != popupMinW {
		t.Errorf("popupInner(20) = %d, want %d", got, popupMinW)
	}
	narrow := model{width: 10}
	if got := narrow.popupInner(50); got != popupMinW {
		t.Errorf("on a tiny terminal popupInner = %d, want %d", got, popupMinW)
	}
}

// Deep find keeps the full screen; every other menu floats.
func TestPickerPopupKinds(t *testing.T) {
	for _, k := range []pickerKind{pickOpenWith, pickCopy, pickBookmarks} {
		if !(model{pickerKind: k}).pickerPopup() {
			t.Errorf("picker kind %v should float", k)
		}
	}
	if (model{pickerKind: pickFind}).pickerPopup() {
		t.Error("deep find should stay full-screen")
	}
}

func newTestPicker(kind pickerKind, items []string, w, h int) model {
	m := model{width: w, height: h, mode: modePicker, pickerKind: kind}
	m.pickerTitle = "menu"
	m.pickerAll = items
	m.pickerApplyFilter()
	return m
}

func TestPickerBoxRendersRows(t *testing.T) {
	m := newTestPicker(pickCopy, []string{"absolute path", "relative path", "file name"}, 100, 28)
	box := m.pickerBox()
	assertRectangular(t, box)

	out := strip(box)
	for _, want := range []string{"menu", "absolute path", "file name", "enter select"} {
		if !strings.Contains(out, want) {
			t.Errorf("picker box missing %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "▶ absolute path") {
		t.Errorf("cursor should mark the first row:\n%s", out)
	}
}

// The box must not resize as the filter narrows the list, or it jumps around
// under the cursor while you type.
func TestPickerBoxKeepsHeightWhileFiltering(t *testing.T) {
	m := newTestPicker(pickBookmarks, []string{"alpha", "beta", "gamma", "delta"}, 100, 28)
	before := lipgloss.Height(m.pickerBox())

	m.ti.SetValue("alpha")
	m.pickerApplyFilter()
	if len(m.pickerRows) != 1 {
		t.Fatalf("filter matched %d rows, want 1", len(m.pickerRows))
	}
	if after := lipgloss.Height(m.pickerBox()); after != before {
		t.Errorf("box height changed from %d to %d while filtering", before, after)
	}
}

func TestPickerBoxNoMatches(t *testing.T) {
	m := newTestPicker(pickCopy, []string{"alpha", "beta"}, 100, 28)
	m.ti.SetValue("zzzz")
	m.pickerApplyFilter()
	if out := strip(m.pickerBox()); !strings.Contains(out, "no matches") {
		t.Errorf("expected a no-matches line:\n%s", out)
	}
}

// A floating picker shows at most popupMaxRows and scrolls beyond that.
func TestPickerPopupHeightCapped(t *testing.T) {
	items := make([]string, 40)
	for i := range items {
		items[i] = strings.Repeat("x", 8)
	}
	m := newTestPicker(pickOpenWith, items, 100, 40)
	if got := m.pickerHeight(); got != popupMaxRows {
		t.Errorf("picker height = %d, want %d", got, popupMaxRows)
	}

	short := newTestPicker(pickOpenWith, items, 100, 12)
	if got := short.pickerHeight(); got != 6 { // height - 6
		t.Errorf("on a short terminal height = %d, want 6", got)
	}
}

func TestPickerFullViewStaysFullScreen(t *testing.T) {
	m := newTestPicker(pickFind, []string{"a.go", "b.go"}, 80, 24)
	out := strip(m.pickerView())
	if strings.Contains(out, "╭") {
		t.Errorf("deep find should not be boxed:\n%s", out)
	}
}

func TestHelpLayoutColumns(t *testing.T) {
	wide := model{width: 130, height: 40}
	cols, _, _, rows, _ := wide.helpLayout()
	if cols != 2 {
		t.Errorf("wide terminal: cols = %d, want 2", cols)
	}
	want := (len(helpBindings()) + 1) / 2
	if rows != want {
		t.Errorf("rows = %d, want %d", rows, want)
	}

	narrow := model{width: 70, height: 40}
	if cols, _, _, _, _ := narrow.helpLayout(); cols != 1 {
		t.Errorf("narrow terminal: cols = %d, want 1", cols)
	}
}

func TestHelpBoxFitsAndScrolls(t *testing.T) {
	m := model{width: 130, height: 30}
	box := m.helpBox()
	assertRectangular(t, box)
	if h := lipgloss.Height(box); h > m.height {
		t.Errorf("help box is %d rows, taller than the %d-row terminal", h, m.height)
	}
	out := strip(box)
	if !strings.Contains(out, "keybindings") || !strings.Contains(out, "quit") {
		t.Errorf("two-column help should show the whole list:\n%s", out)
	}

	// Short terminal: the list scrolls and says so.
	short := model{width: 130, height: 14}
	if short.helpTopMax() == 0 {
		t.Fatal("expected the help list to scroll on a 14-row terminal")
	}
	if out := strip(short.helpBox()); !strings.Contains(out, "j/k scroll") {
		t.Errorf("scrollable help should hint at scrolling:\n%s", out)
	}
	if h := lipgloss.Height(short.helpBox()); h > short.height {
		t.Errorf("help box is %d rows, taller than the %d-row terminal", h, short.height)
	}
}

// The help window swallows browse keys and clamps its own scrolling.
func TestHelpScrollKeys(t *testing.T) {
	m := model{width: 130, height: 12, showHelp: true}
	m.panes[0].dir, m.panes[1].dir = "/tmp", "/tmp"

	next, _ := m.updateBrowse(keyRune('j'))
	if next.(model).helpTop != 1 {
		t.Errorf("j should scroll down, helpTop = %d", next.(model).helpTop)
	}

	// Scrolling up from the top stays at the top.
	up, _ := m.updateBrowse(keyRune('k'))
	if up.(model).helpTop != 0 {
		t.Errorf("k at the top should stay at 0, got %d", up.(model).helpTop)
	}

	// G goes to the end and no further.
	end, _ := m.updateBrowse(keyRune('G'))
	if got := end.(model).helpTop; got != m.helpTopMax() {
		t.Errorf("G should jump to %d, got %d", m.helpTopMax(), got)
	}
	past := end.(model)
	for i := 0; i < 20; i++ {
		n, _ := past.updateBrowse(keyRune('j'))
		past = n.(model)
	}
	if past.helpTop != m.helpTopMax() {
		t.Errorf("scrolling past the end reached %d, want %d", past.helpTop, m.helpTopMax())
	}

	// A browse command must not leak through while help is open.
	yanked, _ := m.updateBrowse(keyRune('y'))
	if yanked.(model).clip != nil {
		t.Error("y must not yank while the help window is open")
	}
	closed, _ := m.updateBrowse(keyRune('?'))
	if closed.(model).showHelp {
		t.Error("? should close the help window")
	}
}
