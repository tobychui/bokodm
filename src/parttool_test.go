package main

import "testing"

func TestParsePartedOutput(t *testing.T) {
	// Real-world shaped output of:
	//   parted -sm /dev/sdb unit B print free
	out := "BYT;\n" +
		"/dev/sdb:2000398934016B:scsi:512:4096:gpt:ATA ST2000DL003-9VT1:;\n" +
		"1:17408B:1048575B:1031168B:free;\n" +
		"1:1048576B:1000000487423B:999999438848B:ext4:data:;\n" +
		"1:1000000487424B:2000398917119B:1000398429696B:free;\n"

	layout := &PartDiskLayout{
		Name:     "sdb",
		Path:     "/dev/sdb",
		SizeByte: 2000398934016,
	}
	mountpoints := map[string]string{"sdb1": "/media/data"}
	parsePartedOutput(layout, out, mountpoints)

	if layout.Table != "gpt" {
		t.Errorf("expected table gpt, got %s", layout.Table)
	}
	if layout.Model != "ATA ST2000DL003-9VT1" {
		t.Errorf("unexpected model: %s", layout.Model)
	}
	if len(layout.Segments) != 3 {
		t.Fatalf("expected 3 segments, got %d", len(layout.Segments))
	}

	if layout.Segments[0].Type != "free" || layout.Segments[0].SizeByte != 1031168 {
		t.Errorf("segment 0 wrong: %+v", layout.Segments[0])
	}

	part := layout.Segments[1]
	if part.Type != "partition" || part.Number != 1 {
		t.Errorf("segment 1 should be partition 1: %+v", part)
	}
	if part.Path != "/dev/sdb1" {
		t.Errorf("expected /dev/sdb1, got %s", part.Path)
	}
	if part.FsType != "ext4" || part.Label != "data" {
		t.Errorf("fs/label wrong: %+v", part)
	}
	if !part.Mounted || part.MountPoint != "/media/data" {
		t.Errorf("mount state wrong: %+v", part)
	}

	if layout.Segments[2].Type != "free" || layout.Segments[2].EndByte != 2000398917119 {
		t.Errorf("segment 2 wrong: %+v", layout.Segments[2])
	}
}

func TestParsePartedOutputLoopTable(t *testing.T) {
	// md RAID volume formatted directly with ext4 (no partition table):
	// parted reports this as a "loop" label with one pseudo partition
	out := "BYT;\n" +
		"/dev/md0:2000263380992B:md:512:4096:loop:Linux Software RAID Array:;\n" +
		"1:0B:2000263380991B:2000263380992B:ext4::;\n"

	layout := &PartDiskLayout{
		Name:     "md0",
		Path:     "/dev/md0",
		SizeByte: 2000263380992,
	}
	parsePartedOutput(layout, out, map[string]string{"md0": "/media/storage1"})

	if layout.Table != "loop" {
		t.Errorf("expected table loop, got %s", layout.Table)
	}
	if len(layout.Segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(layout.Segments))
	}
	segment := layout.Segments[0]
	if segment.Path != "/dev/md0" {
		t.Errorf("loop segment should point at the device itself, got %s", segment.Path)
	}
	if segment.Number != 0 {
		t.Errorf("loop segment number should be 0, got %d", segment.Number)
	}
	if !segment.Mounted || segment.MountPoint != "/media/storage1" {
		t.Errorf("mount state wrong: %+v", segment)
	}
}

func TestParseMdstatNames(t *testing.T) {
	mdstat := "Personalities : [raid1] [linear] [multipath] [raid0] [raid6] [raid5] [raid4] [raid10]\n" +
		"md0 : active raid1 sdc[1] sdb[0]\n" +
		"      1953382464 blocks super 1.2 [2/2] [UU]\n" +
		"      bitmap: 0/15 pages [0KB], 65536KB chunk\n" +
		"\n" +
		"md127 : active raid0 sde[0] sdf[1]\n" +
		"      100000 blocks\n" +
		"\n" +
		"unused devices: <none>\n"

	names := parseMdstatNames(mdstat)
	if len(names) != 2 || names[0] != "md0" || names[1] != "md127" {
		t.Errorf("expected [md0 md127], got %v", names)
	}
}

func TestPartitionDevPath(t *testing.T) {
	if got := partitionDevPath("sdb", 1); got != "/dev/sdb1" {
		t.Errorf("sdb: got %s", got)
	}
	if got := partitionDevPath("nvme0n1", 2); got != "/dev/nvme0n1p2" {
		t.Errorf("nvme0n1: got %s", got)
	}
	if got := partitionDevPath("mmcblk0", 1); got != "/dev/mmcblk0p1" {
		t.Errorf("mmcblk0: got %s", got)
	}
}

func TestRecreatePartEnd(t *testing.T) {
	const alignment = int64(1 << 20)

	if got := recreatePartEnd(alignment, 5120, 0, alignment); got != alignment+5120*alignment-1 {
		t.Fatalf("fixed-size partition end should be inclusive, got %d", got)
	}

	diskSize := int64(2000263380992)
	if got := recreatePartEnd(alignment, 0, diskSize, alignment); got != diskSize-alignment-1 {
		t.Fatalf("rest-of-disk partition end should leave tail room, got %d", got)
	}
}

func TestFindCreatedRecreateSegment(t *testing.T) {
	layout := &PartDiskLayout{Segments: []PartSegment{
		{Type: "partition", Number: 1, StartByte: 1048576, EndByte: 5369758207},
		{Type: "free", StartByte: 5369758208, EndByte: 5369762303},
		{Type: "partition", Number: 2, StartByte: 5369762304, EndByte: 2000262332415},
	}}

	segment := findCreatedRecreateSegment(layout, 5369757696, 2000262332415, map[int]bool{1: true})
	if segment == nil {
		t.Fatal("expected to find the aligned second partition")
	}
	if segment.Number != 2 {
		t.Fatalf("expected partition 2, got %d", segment.Number)
	}
}
