package main

import (
	"strings"
	"testing"
)

// Recorded lsblk --json -b output: a loop device, the internal NVMe disk, a
// partitioned USB stick (one partition mounted, one not), an unpartitioned SD
// card, and a LUKS container that can't be opened from here.
const lsblkFixture = `{
  "blockdevices": [
    {"path":"/dev/loop0","name":"loop0","label":null,"size":4096,"fstype":"squashfs",
     "mountpoint":"/snap/bare/5","rm":false,"hotplug":false,"type":"loop",
     "vendor":null,"model":null,"fsavail":0},
    {"path":"/dev/nvme0n1","name":"nvme0n1","label":null,"size":1024209543168,"fstype":null,
     "mountpoint":null,"rm":false,"hotplug":false,"type":"disk",
     "vendor":null,"model":"Samsung SSD 990","fsavail":null,
     "children":[
       {"path":"/dev/nvme0n1p2","name":"nvme0n1p2","label":null,"size":262144000000,"fstype":"ext4",
        "mountpoint":"/","rm":false,"hotplug":false,"type":"part",
        "vendor":null,"model":null,"fsavail":100000000000}]},
    {"path":"/dev/sdb","name":"sdb","label":null,"size":31017730048,"fstype":null,
     "mountpoint":null,"rm":true,"hotplug":true,"type":"disk",
     "vendor":"SanDisk ","model":"Ultra USB 3.0   ","fsavail":null,
     "children":[
       {"path":"/dev/sdb1","name":"sdb1","label":"FIELD","size":16000000000,"fstype":"vfat",
        "mountpoint":"/run/media/kz/FIELD","rm":false,"hotplug":false,"type":"part",
        "vendor":null,"model":null,"fsavail":4000000000},
       {"path":"/dev/sdb2","name":"sdb2","label":null,"size":15017730048,"fstype":"ntfs",
        "mountpoint":null,"rm":false,"hotplug":false,"type":"part",
        "vendor":null,"model":null,"fsavail":null},
       {"path":"/dev/sdb3","name":"sdb3","label":null,"size":1048576,"fstype":"crypto_LUKS",
        "mountpoint":null,"rm":false,"hotplug":false,"type":"part",
        "vendor":null,"model":null,"fsavail":null}]},
    {"path":"/dev/mmcblk0","name":"mmcblk0","label":"CARD","size":64000000000,"fstype":"exfat",
     "mountpoint":null,"rm":true,"hotplug":true,"type":"disk",
     "vendor":null,"model":null,"fsavail":null}
  ]
}`

func TestParseDrivesPicksExternalLeaves(t *testing.T) {
	ds, err := parseDrives([]byte(lsblkFixture))
	if err != nil {
		t.Fatalf("parseDrives: %v", err)
	}
	var got []string
	for _, d := range ds {
		got = append(got, d.dev)
	}
	want := []string{"/dev/mmcblk0", "/dev/sdb1", "/dev/sdb2"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("devices = %v, want %v (internal disk, loop and LUKS must be skipped)", got, want)
	}
}

func TestParseDrivesFields(t *testing.T) {
	ds, err := parseDrives([]byte(lsblkFixture))
	if err != nil {
		t.Fatalf("parseDrives: %v", err)
	}
	by := map[string]drive{}
	for _, d := range ds {
		by[d.dev] = d
	}

	mounted := by["/dev/sdb1"]
	if mounted.label != "FIELD" {
		t.Errorf("label = %q, want the filesystem label FIELD", mounted.label)
	}
	if !mounted.mounted() || mounted.mount != "/run/media/kz/FIELD" {
		t.Errorf("mount = %q, want /run/media/kz/FIELD", mounted.mount)
	}
	if mounted.parent != "/dev/sdb" {
		t.Errorf("parent = %q, want the whole disk /dev/sdb", mounted.parent)
	}
	if mounted.free != 4000000000 {
		t.Errorf("free = %d, want 4000000000", mounted.free)
	}

	// No label: fall back to the parent disk's vendor + model, whitespace collapsed.
	unmounted := by["/dev/sdb2"]
	if unmounted.label != "SanDisk Ultra USB 3.0" {
		t.Errorf("label = %q, want vendor+model fallback", unmounted.label)
	}
	if unmounted.mounted() {
		t.Errorf("sdb2 should not be mounted")
	}
	if unmounted.free != -1 {
		t.Errorf("free = %d, want -1 (unknown) for an unmounted drive", unmounted.free)
	}

	// An unpartitioned card is its own whole-disk device, so eject powers it off.
	card := by["/dev/mmcblk0"]
	if card.parent != "/dev/mmcblk0" {
		t.Errorf("parent = %q, want itself for an unpartitioned disk", card.parent)
	}
}

// Older util-linux quotes numbers and booleans in --json output.
func TestParseDrivesAcceptsQuotedValues(t *testing.T) {
	const old = `{"blockdevices":[
	  {"path":"/dev/sdc1","name":"sdc1","label":"OLD","size":"2048","fstype":"vfat",
	   "mountpoint":"/mnt/old","rm":"1","hotplug":"0","type":"part",
	   "vendor":null,"model":null,"fsavail":"1024"}]}`
	ds, err := parseDrives([]byte(old))
	if err != nil {
		t.Fatalf("parseDrives: %v", err)
	}
	if len(ds) != 1 {
		t.Fatalf("got %d drives, want 1", len(ds))
	}
	if ds[0].size != 2048 || ds[0].free != 1024 {
		t.Errorf("size/free = %d/%d, want 2048/1024", ds[0].size, ds[0].free)
	}
}

func TestParseDrivesRejectsGarbage(t *testing.T) {
	if _, err := parseDrives([]byte("not json")); err == nil {
		t.Fatal("expected an error for non-JSON lsblk output")
	}
}

func TestMountableFS(t *testing.T) {
	for _, fs := range []string{"ext4", "vfat", "ntfs", "exfat", "btrfs"} {
		if !mountableFS(fs) {
			t.Errorf("%s should be mountable", fs)
		}
	}
	for _, fs := range []string{"", "swap", "squashfs", "crypto_LUKS", "LVM2_member"} {
		if mountableFS(fs) {
			t.Errorf("%s should not be offered", fs)
		}
	}
}

// The drives window must not react to browse keys, and must ignore everything
// while an action is in flight.
func TestDrivesModeIgnoresBrowseKeys(t *testing.T) {
	m := model{mode: modeDrives, width: 80, height: 24}
	m.drives = []drive{{dev: "/dev/sdb1", label: "A"}, {dev: "/dev/sdb2", label: "B"}}

	for _, key := range []rune{'y', 'p', 'z', 'd', 'x', 'c'} {
		next, _ := m.updateDrives(keyRune(key))
		got := next.(model)
		if got.mode != modeDrives {
			t.Errorf("key %q left the drives window", key)
		}
		if got.clip != nil {
			t.Errorf("key %q touched the clipboard", key)
		}
	}

	moved, _ := m.updateDrives(keyRune('j'))
	if moved.(model).driveCursor != 1 {
		t.Error("j should move the drive cursor")
	}

	busy := m
	busy.driveBusy = "Ejecting…"
	frozen, _ := busy.updateDrives(keyRune('j'))
	if frozen.(model).driveCursor != 0 {
		t.Error("keys must be ignored while an action is running")
	}
}

func TestDrivesCursorClamps(t *testing.T) {
	m := model{mode: modeDrives}
	m.drives = []drive{{dev: "/dev/sdb1"}, {dev: "/dev/sdb2"}}
	m.moveDriveCursor(10)
	if m.driveCursor != 1 {
		t.Errorf("cursor = %d, want 1", m.driveCursor)
	}
	m.moveDriveCursor(-10)
	if m.driveCursor != 0 {
		t.Errorf("cursor = %d, want 0", m.driveCursor)
	}

	empty := model{}
	if _, ok := empty.currentDrive(); ok {
		t.Error("currentDrive must report no drive when the list is empty")
	}
}

func TestDrivesBoxShowsStateAndKeys(t *testing.T) {
	m := model{mode: modeDrives, width: 100, height: 30}
	m.drives = []drive{
		{dev: "/dev/sdb1", label: "FIELD", fstype: "vfat", mount: "/run/media/kz/FIELD", size: 16e9, free: 4e9},
		{dev: "/dev/sdb2", label: "SanDisk Ultra", fstype: "ntfs", size: 15e9, free: -1},
	}
	out := strip(m.drivesBox())

	for _, want := range []string{"drives", "FIELD", "SanDisk Ultra", "not mounted", "u unmount", "e eject"} {
		if !strings.Contains(out, want) {
			t.Errorf("drives box missing %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "free of") {
		t.Errorf("detail line should report free space:\n%s", out)
	}

	// Every line inside the border must be the same width, or the box is ragged.
	lines := strings.Split(out, "\n")
	for i, l := range lines[1:] {
		if len([]rune(l)) != len([]rune(lines[0])) {
			t.Fatalf("line %d width %d != %d:\n%s", i+1, len([]rune(l)), len([]rune(lines[0])), out)
		}
	}
}

func TestDrivesBoxEmpty(t *testing.T) {
	m := model{mode: modeDrives, width: 80, height: 24}
	if out := strip(m.drivesBox()); !strings.Contains(out, "no external drives found") {
		t.Errorf("empty drives box should say so:\n%s", out)
	}
}

// Unmounting the drive a pane is browsing must move that pane somewhere valid.
func TestEvacuateLeavesUnmountedDir(t *testing.T) {
	dir := t.TempDir()
	m := newModel(dir)
	m.panes[0].dir = "/run/media/kz/FIELD/photos"
	m.panes[1].dir = dir

	m.evacuate("/run/media/kz/FIELD")

	if strings.HasPrefix(m.panes[0].dir, "/run/media/kz/FIELD") {
		t.Errorf("pane 0 still inside the unmounted drive: %s", m.panes[0].dir)
	}
	if m.panes[1].dir != dir {
		t.Errorf("pane 1 moved but shouldn't have: %s", m.panes[1].dir)
	}
}
