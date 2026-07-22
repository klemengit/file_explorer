package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// drive is one external volume shown in the M window: a partition (or a whole
// unpartitioned disk) that lsblk reports as removable or hotplug.
type drive struct {
	dev    string // partition device, e.g. /dev/sdb1
	parent string // whole-disk device, e.g. /dev/sdb — what gets powered off
	label  string // filesystem label, else vendor+model, else the device name
	fstype string
	mount  string // mountpoint, empty when not mounted
	size   int64  // bytes, -1 when unknown
	free   int64  // bytes available, -1 when unknown or not mounted
}

func (d drive) mounted() bool { return d.mount != "" }

// lsblkCols are the columns listDrives asks lsblk for, in JSON form.
const lsblkCols = "PATH,NAME,LABEL,SIZE,FSTYPE,MOUNTPOINT,RM,HOTPLUG,TYPE,VENDOR,MODEL,FSAVAIL"

// flexInt decodes a number that lsblk emits as a JSON number with -b, but that
// older util-linux versions quote as a string. Missing/unparsable becomes -1
// ("unknown") rather than an error, so one odd device can't break the listing.
type flexInt int64

func (f *flexInt) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*f = -1
		return nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		*f = -1
		return nil
	}
	*f = flexInt(n)
	return nil
}

// flexBool decodes lsblk's booleans, which are real JSON booleans in
// util-linux >= 2.37 and quoted "0"/"1" before that.
type flexBool bool

func (f *flexBool) UnmarshalJSON(b []byte) error {
	switch strings.Trim(string(b), `"`) {
	case "true", "1":
		*f = true
	default:
		*f = false
	}
	return nil
}

// lsblkNode is one entry of lsblk's JSON tree; disks carry their partitions in
// Children.
type lsblkNode struct {
	Path       string      `json:"path"`
	Name       string      `json:"name"`
	Label      string      `json:"label"`
	FSType     string      `json:"fstype"`
	Mountpoint string      `json:"mountpoint"`
	Vendor     string      `json:"vendor"`
	Model      string      `json:"model"`
	Type       string      `json:"type"`
	Size       flexInt     `json:"size"`
	FSAvail    flexInt     `json:"fsavail"`
	RM         flexBool    `json:"rm"`
	Hotplug    flexBool    `json:"hotplug"`
	Children   []lsblkNode `json:"children"`
}

// listDrives returns the external volumes currently attached.
func listDrives() ([]drive, error) {
	out, err := exec.Command("lsblk", "--json", "-b", "-o", lsblkCols).Output()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, errors.New("lsblk not found — install util-linux")
		}
		return nil, fmt.Errorf("lsblk: %v", err)
	}
	return parseDrives(out)
}

// parseDrives turns lsblk JSON into the drive list. Split out from listDrives
// so it can be tested against recorded output.
func parseDrives(data []byte) ([]drive, error) {
	var doc struct {
		Blockdevices []lsblkNode `json:"blockdevices"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("lsblk output: %v", err)
	}
	var ds []drive
	for _, n := range doc.Blockdevices {
		collectDrives(n, lsblkNode{}, false, &ds)
	}
	// Device order keeps a disk's partitions together and is stable between
	// refreshes, unlike mount order.
	sort.Slice(ds, func(i, j int) bool { return ds[i].dev < ds[j].dev })
	return ds, nil
}

// collectDrives walks one lsblk subtree. A node counts as external if it — or
// any ancestor — is flagged removable or hotplug, since partitions of a USB
// disk don't always repeat the flag. Only leaves are listed: a partitioned disk
// contributes its partitions, not itself.
func collectDrives(n, parent lsblkNode, parentExternal bool, out *[]drive) {
	if n.Type == "loop" {
		return
	}
	external := parentExternal || bool(n.RM) || bool(n.Hotplug)
	if len(n.Children) > 0 {
		for _, c := range n.Children {
			collectDrives(c, n, external, out)
		}
		return
	}
	if !external || !mountableFS(n.FSType) {
		return
	}
	whole := parent.Path
	if whole == "" {
		whole = n.Path // an unpartitioned stick is its own whole-disk device
	}
	d := drive{
		dev:    n.Path,
		parent: whole,
		label:  driveLabel(n, parent),
		fstype: n.FSType,
		mount:  n.Mountpoint,
		size:   int64(n.Size),
		free:   -1,
	}
	if d.mounted() {
		d.free = int64(n.FSAvail)
	}
	*out = append(*out, d)
}

// mountableFS reports whether a filesystem type is something the window can
// offer to open. Container members (LUKS, LVM, RAID) need a separate unlock or
// assembly step, and swap/squashfs aren't browsable.
func mountableFS(fs string) bool {
	switch fs {
	case "", "swap", "squashfs", "crypto_LUKS", "LVM2_member", "linux_raid_member", "zfs_member":
		return false
	}
	return true
}

// driveLabel picks the friendliest name available: the filesystem label, else
// the drive's vendor and model, else the bare device name.
func driveLabel(n, parent lsblkNode) string {
	if s := strings.TrimSpace(n.Label); s != "" {
		return s
	}
	for _, src := range []lsblkNode{n, parent} {
		name := strings.Join(strings.Fields(src.Vendor+" "+src.Model), " ")
		if name != "" {
			return name
		}
	}
	if n.Path != "" {
		return filepath.Base(n.Path)
	}
	return n.Name
}

// driveResultMsg reports the outcome of a mount/unmount/eject back to the
// update loop, so the window stays responsive while the command runs.
type driveResultMsg struct {
	text  string // status line to show when err is nil
	dev   string
	freed string // mountpoint released, so open panes can be moved out of it
	enter bool   // navigate into the drive once it is mounted
	err   error
}

// run executes a command and folds its output into the error, since udisksctl
// explains failures (busy, not authorized) on stderr.
func run(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err == nil {
		return nil
	}
	if errors.Is(err, exec.ErrNotFound) {
		return fmt.Errorf("%s not found", name)
	}
	if msg := firstLine(string(out)); msg != "" {
		return errors.New(msg)
	}
	return err
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

func haveUdisks() bool {
	_, err := exec.LookPath("udisksctl")
	return err == nil
}

// mountDriveCmd mounts d in the background. udisksctl picks the mountpoint
// (/run/media/<user>/<label>) and works without root via polkit.
func mountDriveCmd(d drive, enter bool) tea.Cmd {
	return func() tea.Msg {
		if !haveUdisks() {
			return driveResultMsg{dev: d.dev, enter: enter,
				err: errors.New("udisksctl not found — can't mount without it")}
		}
		if err := run("udisksctl", "mount", "-b", d.dev); err != nil {
			return driveResultMsg{dev: d.dev, enter: enter, err: err}
		}
		return driveResultMsg{dev: d.dev, enter: enter, text: "Mounted " + d.label}
	}
}

func unmountDriveCmd(d drive) tea.Cmd {
	return func() tea.Msg {
		if err := unmountDrive(d); err != nil {
			return driveResultMsg{dev: d.dev, err: err}
		}
		return driveResultMsg{dev: d.dev, freed: d.mount, text: "Unmounted " + d.label}
	}
}

// ejectDriveCmd unmounts and then powers the device down, so the drive can be
// unplugged. If power-off isn't supported the unmount still stands and the
// status says so rather than claiming a safe removal.
func ejectDriveCmd(d drive) tea.Cmd {
	return func() tea.Msg {
		off, err := ejectDrive(d)
		if err != nil {
			return driveResultMsg{dev: d.dev, err: err}
		}
		text := "Ejected " + d.label + " — safe to remove"
		if !off {
			text = "Unmounted " + d.label + " (power-off not supported)"
		}
		return driveResultMsg{dev: d.dev, freed: d.mount, text: text}
	}
}

// openDrives enters the external-drives window. Unlike the pickers there is no
// filter input, so its keys (u, e, r) are plain letters.
func (m model) openDrives() (tea.Model, tea.Cmd) {
	m.mode = modeDrives
	m.drives = nil // drop the previous listing so the cursor starts at the top
	m.driveCursor = 0
	m.driveBusy = ""
	m.driveNote, m.driveNoteLv = "", lvlInfo
	m.refreshDrives()
	return m, nil
}

// refreshDrives re-reads the drive list, keeping the cursor on the same device
// where it still exists.
func (m *model) refreshDrives() {
	var on string
	if d, ok := m.currentDrive(); ok {
		on = d.dev
	}
	ds, err := listDrives()
	if err != nil {
		m.drives = nil
		m.driveCursor = 0
		m.driveNote, m.driveNoteLv = err.Error(), lvlErr
		return
	}
	m.drives = ds
	m.driveCursor = 0
	for i, d := range ds {
		if d.dev == on {
			m.driveCursor = i
			break
		}
	}
}

func (m model) currentDrive() (drive, bool) {
	if m.driveCursor < 0 || m.driveCursor >= len(m.drives) {
		return drive{}, false
	}
	return m.drives[m.driveCursor], true
}

func (m *model) moveDriveCursor(delta int) {
	m.driveCursor += delta
	if m.driveCursor >= len(m.drives) {
		m.driveCursor = len(m.drives) - 1
	}
	if m.driveCursor < 0 {
		m.driveCursor = 0
	}
}

// updateDrives handles the drives window. Only its own keys are live here —
// browse commands like y/p/z stay inert while it is open — and every key is
// ignored while an action is still running.
func (m model) updateDrives(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.driveBusy != "" {
		return m, nil
	}
	switch msg.String() {
	case "esc", "q", "M":
		m.mode = modeBrowse
	case "j", "down":
		m.moveDriveCursor(1)
	case "k", "up":
		m.moveDriveCursor(-1)
	case "g", "home":
		m.driveCursor = 0
	case "G", "end":
		m.moveDriveCursor(len(m.drives))
	case "r":
		m.driveNote = ""
		m.refreshDrives()
	case "enter", "l", "right":
		d, ok := m.currentDrive()
		if !ok {
			break
		}
		if d.mounted() {
			m.mode = modeBrowse
			m.enterDir(d.mount)
			m.setStatus(lvlInfo, "%s — %s", d.label, abbrevHome(d.mount))
			break
		}
		m.driveNote = ""
		m.driveBusy = "Mounting " + d.label + "…"
		return m, mountDriveCmd(d, true)
	case "u":
		d, ok := m.currentDrive()
		if !ok {
			break
		}
		if !d.mounted() {
			m.driveNote, m.driveNoteLv = d.label+" is not mounted", lvlWarn
			break
		}
		m.driveNote = ""
		m.driveBusy = "Unmounting " + d.label + "…"
		return m, unmountDriveCmd(d)
	case "e":
		d, ok := m.currentDrive()
		if !ok {
			break
		}
		m.driveNote = ""
		m.driveBusy = "Ejecting " + d.label + "…"
		return m, ejectDriveCmd(d)
	}
	return m, nil
}

// applyDriveResult folds a finished mount/unmount/eject back into the model:
// it refreshes the listing, moves any pane that was sitting on a mountpoint
// that just went away, and follows a mount that was asked to be entered.
func (m model) applyDriveResult(msg driveResultMsg) (tea.Model, tea.Cmd) {
	m.driveBusy = ""
	if msg.err != nil {
		m.driveNote, m.driveNoteLv = msg.err.Error(), lvlErr
		m.refreshDrives()
		return m, nil
	}
	m.driveNote, m.driveNoteLv = msg.text, lvlInfo
	if msg.freed != "" {
		m.evacuate(msg.freed)
	}
	m.refreshDrives()

	if msg.enter {
		for i, d := range m.drives {
			if d.dev == msg.dev && d.mounted() {
				m.driveCursor = i
				m.mode = modeBrowse
				m.enterDir(d.mount)
				m.setStatus(lvlInfo, "%s — %s", d.label, abbrevHome(d.mount))
				break
			}
		}
	}
	return m, nil
}

// evacuate moves any pane browsing inside the mountpoint mp (which has just
// been unmounted) back to the user's home directory, so no pane is left
// showing a directory that no longer exists.
func (m *model) evacuate(mp string) {
	fallback, err := os.UserHomeDir()
	if err != nil || fallback == "" {
		fallback = "/"
	}
	for i := range m.panes {
		dir := m.panes[i].dir
		if dir != mp && !strings.HasPrefix(dir, mp+"/") {
			continue
		}
		m.panes[i].clearSelection()
		if err := m.panes[i].enterDir(fallback, m.filterQuery()); err != nil {
			m.setStatus(lvlErr, "%v", err)
		}
	}
}

func unmountDrive(d drive) error {
	if !d.mounted() {
		return nil
	}
	if haveUdisks() {
		return run("udisksctl", "unmount", "-b", d.dev)
	}
	return run("umount", d.dev)
}

// ejectDrive unmounts d and powers off its whole-disk device. It reports
// whether the power-off actually happened; optical drives and a few bridges
// reject power-off, in which case `eject` is tried instead.
func ejectDrive(d drive) (bool, error) {
	if err := unmountDrive(d); err != nil {
		return false, err
	}
	if haveUdisks() {
		if err := run("udisksctl", "power-off", "-b", d.parent); err == nil {
			return true, nil
		}
	}
	if _, err := exec.LookPath("eject"); err == nil {
		if err := run("eject", d.parent); err == nil {
			return true, nil
		}
	}
	return false, nil
}
