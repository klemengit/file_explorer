package main

import (
	"os/exec"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
)

// app is one candidate in the "open with" picker. argv[0] is the binary looked
// up on PATH; the selected file's path is appended to argv when launched.
// terminal apps need the tty and are run via tea.ExecProcess (suspending the
// TUI); the rest are launched detached in the background.
type app struct {
	label    string
	argv     []string
	terminal bool
}

// curatedApps is the master list of "open with" candidates. Only those whose
// binary is found on PATH are shown, so the picker adapts to what's installed.
// The label is what you see and fuzzy-search; where the binary name differs
// from the app name it's noted in parentheses so either matches a search.
var curatedApps = []app{
	{"Default app (xdg-open)", []string{"xdg-open"}, false},

	// terminal editors / pagers
	{"nvim", []string{"nvim"}, true},
	{"vim", []string{"vim"}, true},
	{"vi", []string{"vi"}, true},
	{"nano", []string{"nano"}, true},
	{"emacs (terminal)", []string{"emacs", "-nw"}, true},
	{"Helix (hx)", []string{"hx"}, true},
	{"micro", []string{"micro"}, true},
	{"Kakoune (kak)", []string{"kak"}, true},
	{"less (pager)", []string{"less"}, true},
	{"bat (pager)", []string{"bat"}, true},

	// terminal file managers
	{"ranger", []string{"ranger"}, true},
	{"nnn", []string{"nnn"}, true},
	{"lf", []string{"lf"}, true},
	{"vifm", []string{"vifm"}, true},

	// GUI editors / IDEs
	{"VS Code (code)", []string{"code"}, false},
	{"VSCodium (codium)", []string{"codium"}, false},
	{"Sublime Text (subl)", []string{"subl"}, false},
	{"Zed (zed)", []string{"zed"}, false},
	{"gedit", []string{"gedit"}, false},
	{"Kate (kate)", []string{"kate"}, false},
	{"GNOME Text Editor (gnome-text-editor)", []string{"gnome-text-editor"}, false},
	{"Mousepad (mousepad)", []string{"mousepad"}, false},
	{"Geany (geany)", []string{"geany"}, false},

	// browsers
	{"Firefox (firefox)", []string{"firefox"}, false},
	{"Google Chrome (google-chrome)", []string{"google-chrome"}, false},
	{"Chromium (chromium)", []string{"chromium"}, false},
	{"Brave (brave-browser)", []string{"brave-browser"}, false},

	// image viewers / editors
	{"Eye of GNOME (eog)", []string{"eog"}, false},
	{"feh", []string{"feh"}, false},
	{"Gwenview (gwenview)", []string{"gwenview"}, false},
	{"nomacs", []string{"nomacs"}, false},
	{"sxiv", []string{"sxiv"}, false},
	{"GIMP (gimp)", []string{"gimp"}, false},
	{"Krita (krita)", []string{"krita"}, false},
	{"Inkscape (inkscape)", []string{"inkscape"}, false},

	// media players
	{"VLC (vlc)", []string{"vlc"}, false},
	{"mpv", []string{"mpv"}, false},
	{"Totem (totem)", []string{"totem"}, false},
	{"Celluloid (celluloid)", []string{"celluloid"}, false},
	{"SMPlayer (smplayer)", []string{"smplayer"}, false},
	{"Audacity (audacity)", []string{"audacity"}, false},

	// documents / PDF / office
	{"Evince (evince)", []string{"evince"}, false},
	{"Zathura (zathura)", []string{"zathura"}, false},
	{"Okular (okular)", []string{"okular"}, false},
	{"MuPDF (mupdf)", []string{"mupdf"}, false},
	{"LibreOffice (libreoffice)", []string{"libreoffice"}, false},

	// GUI file managers
	{"Files / Nautilus (nautilus)", []string{"nautilus"}, false},
	{"Thunar (thunar)", []string{"thunar"}, false},
	{"Dolphin (dolphin)", []string{"dolphin"}, false},
	{"Nemo (nemo)", []string{"nemo"}, false},
	{"PCManFM (pcmanfm)", []string{"pcmanfm"}, false},

	// misc
	{"Blender (blender)", []string{"blender"}, false},
	{"Obsidian (obsidian)", []string{"obsidian"}, false},
	{"Meld (meld)", []string{"meld"}, false},
}

// installedApps returns the curated apps whose binary is present on PATH.
func installedApps() []app {
	out := make([]app, 0, len(curatedApps))
	for _, a := range curatedApps {
		if _, err := exec.LookPath(a.argv[0]); err == nil {
			out = append(out, a)
		}
	}
	return out
}

// openOpenWith builds the "open with" picker for target: a searchable list of
// installed apps plus a trailing "Custom command…" entry that falls back to the
// typed-command prompt.
func (m model) openOpenWith(target string) (tea.Model, tea.Cmd) {
	apps := installedApps()
	items := make([]string, 0, len(apps)+1)
	for _, a := range apps {
		items = append(items, a.label)
	}
	items = append(items, "Custom command…")

	m.openWith = apps
	m.openWithTarget = target
	m.pickerKind = pickOpenWith
	m.pickerTitle = "open with  ·  " + filepath.Base(target)
	m.pickerAll = items
	m.pickerCursor = 0
	m.pickerTop = 0
	m.mode = modePicker
	m.ti.Prompt = "/ "
	m.ti.SetValue("")
	m.ti.Placeholder = ""
	m.pickerApplyFilter()
	return m, m.ti.Focus()
}

// launchApp runs a chosen app on target. Terminal apps take over the screen via
// tea.ExecProcess (reloading the listing when they exit); GUI apps are detached.
func (m *model) launchApp(a app, target string) tea.Cmd {
	args := append(append([]string{}, a.argv[1:]...), target)
	if a.terminal {
		c := exec.Command(a.argv[0], args...)
		return tea.ExecProcess(c, func(err error) tea.Msg { return editFinishedMsg{err} })
	}
	if err := openDetached(a.argv[0], args...); err != nil {
		m.setStatus(lvlErr, "%v", err)
	}
	return nil
}
