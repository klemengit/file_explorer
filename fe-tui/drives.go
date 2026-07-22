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

	// Set when err is "target is busy": the drive we failed to release, the
	// operation to retry ("unmount" or "eject") and the processes holding it,
	// so the window can name them and offer a forced unmount instead of just
	// repeating the error.
	busy    *drive
	busyOp  string
	holders []holder
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
	if msg := cleanUdisksError(firstLine(string(out))); msg != "" {
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

func unmountDriveCmd(d drive, force bool) tea.Cmd {
	return func() tea.Msg {
		if err := unmountDrive(d, force); err != nil {
			return busyOrPlain(d, "unmount", err)
		}
		return driveResultMsg{dev: d.dev, freed: d.mount, text: "Unmounted " + d.label}
	}
}

// busyOrPlain turns a failed release into a result message, attaching the list
// of processes holding the mount when the failure was "target is busy" — the
// error alone doesn't say who, which is the only useful thing to know.
func busyOrPlain(d drive, op string, err error) driveResultMsg {
	msg := driveResultMsg{dev: d.dev, err: err}
	if isBusyErr(err) {
		msg.busy, msg.busyOp = &d, op
		msg.holders = busyHolders("/proc", d.mount)
	}
	return msg
}

// ejectDriveCmd unmounts and then powers the device down, so the drive can be
// unplugged. If power-off isn't supported the unmount still stands and the
// status says so rather than claiming a safe removal.
func ejectDriveCmd(d drive, force bool) tea.Cmd {
	return func() tea.Msg {
		off, err := ejectDrive(d, force)
		if err != nil {
			return busyOrPlain(d, "eject", err)
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
	m.clearStuck()
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
		// A recheck also re-reads who is holding a stuck drive, so closing
		// the offending program and pressing r clears the warning.
		m.driveNote = ""
		if m.driveStuck != nil {
			m.driveHolders = busyHolders("/proc", m.driveStuck.mount)
		}
		m.refreshDrives()
	case "F":
		// Forced (lazy) unmount, offered only after a busy failure and only
		// on its own key, so it can never happen by accident.
		if m.driveStuck == nil {
			break
		}
		d, op := *m.driveStuck, m.driveStuckOp
		m.clearStuck()
		m.driveNote = ""
		if op == "eject" {
			m.driveBusy = "Force-ejecting " + d.label + "…"
			return m, ejectDriveCmd(d, true)
		}
		m.driveBusy = "Force-unmounting " + d.label + "…"
		return m, unmountDriveCmd(d, true)
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
		m.clearStuck()
		m.driveBusy = "Unmounting " + d.label + "…"
		return m, unmountDriveCmd(d, false)
	case "e":
		d, ok := m.currentDrive()
		if !ok {
			break
		}
		m.driveNote = ""
		m.clearStuck()
		m.driveBusy = "Ejecting " + d.label + "…"
		return m, ejectDriveCmd(d, false)
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
		if msg.busy != nil {
			m.driveStuck, m.driveStuckOp = msg.busy, msg.busyOp
			m.driveHolders = msg.holders
			m.driveNote = fmt.Sprintf("%s is busy — %s", msg.busy.label, describeHolders(msg.holders))
		}
		m.refreshDrives()
		return m, nil
	}
	m.clearStuck()
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

// clearStuck forgets a previous busy failure, so the force-unmount offer
// disappears once the situation has moved on.
func (m *model) clearStuck() {
	m.driveStuck, m.driveStuckOp, m.driveHolders = nil, "", nil
}

// describeHolders summarises who is keeping a mount busy, for the one-line
// note. The per-process detail goes in the lines underneath it.
func describeHolders(hs []holder) string {
	switch len(hs) {
	case 0:
		// Either nothing readable in /proc, or the holder belongs to another
		// user, whose links we can't follow without root.
		return "something still has it open"
	case 1:
		return fmt.Sprintf("%s (%d) is using it", hs[0].name, hs[0].pid)
	default:
		return fmt.Sprintf("%d processes are using it", len(hs))
	}
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

// unmountDrive releases d. With force it asks for a lazy unmount, which
// detaches the filesystem even while processes still hold it — only ever
// reached by an explicit second keypress, since unflushed writes can be lost.
func unmountDrive(d drive, force bool) error {
	if !d.mounted() {
		return nil
	}
	// Never keep the mount busy ourselves: if fe was started from inside the
	// drive, our own working directory pins it.
	if cwd, err := os.Getwd(); err == nil && underMount(cwd, d.mount) {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			os.Chdir(home)
		} else {
			os.Chdir("/")
		}
	}
	if haveUdisks() {
		args := []string{"unmount", "-b", d.dev}
		if force {
			args = append(args, "--force")
		}
		return run("udisksctl", args...)
	}
	args := []string{d.dev}
	if force {
		args = append([]string{"-l"}, args...)
	}
	return run("umount", args...)
}

// ejectDrive unmounts d and powers off its whole-disk device. It reports
// whether the power-off actually happened; optical drives and a few bridges
// reject power-off, in which case `eject` is tried instead.
func ejectDrive(d drive, force bool) (bool, error) {
	if err := unmountDrive(d, force); err != nil {
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
