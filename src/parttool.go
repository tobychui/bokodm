package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"imuslab.com/bokodm/bokodmd/mod/disktool/diskfs"
	"imuslab.com/bokodm/bokodmd/mod/utils"
)

/*
	parttool.go

	This file implements the disk partitioning and formatting tool
	(Tools tab). It exposes a gparted-like workflow over `parted`:
	list disk layouts including free space, create / delete partitions,
	create new partition tables and format partitions.

	Only non-system disks are exposed: the disk hosting /, /boot or the
	swap partition and disks that are RAID members are never listed and
	every mutating call re-validates this server-side.

	Support APIs

	/parttool/list     - GET  layout of all editable (non-system) disks
	/parttool/mklabel  - POST disk=sdX, table=gpt|msdos    (wipes the disk)
	/parttool/mkpart   - POST disk=sdX, start=<bytes>, end=<bytes>, fs=ext4|ntfs|fat32 (optional)
	/parttool/rmpart   - POST disk=sdX, number=N
	/parttool/rmparts  - POST disk=sdX, numbers=[N,...]    (batch delete)
	/parttool/recreate - POST disk=sdX, table=gpt|msdos, parts=[{sizeMB,fs},...]  (wipe + full new layout)
	/parttool/extend   - POST disk=sdX, number=N, end=<bytes>  (grow into following free space + grow fs)
	/parttool/format   - POST partition=sdX1, fs=ext4|ntfs|fat32
*/

// PartSegment is one contiguous region on a disk: either a partition or
// unallocated free space.
type PartSegment struct {
	Type       string `json:"type"` // "partition" or "free"
	Number     int    `json:"number,omitempty"`
	Path       string `json:"path,omitempty"` // /dev/sdX1, partitions only
	StartByte  int64  `json:"startByte"`
	EndByte    int64  `json:"endByte"`
	SizeByte   int64  `json:"sizeByte"`
	FsType     string `json:"fstype,omitempty"`
	Label      string `json:"label,omitempty"`
	MountPoint string `json:"mountPoint,omitempty"`
	Mounted    bool   `json:"mounted"`
}

// PartDiskLayout is the full layout of one editable disk.
type PartDiskLayout struct {
	Name         string        `json:"name"` // sdX or mdX
	Path         string        `json:"path"` // /dev/sdX
	Model        string        `json:"model"`
	SizeByte     int64         `json:"sizeByte"`
	Table        string        `json:"table"`        // gpt / msdos / loop / none
	Busy         bool          `json:"busy"`         // true when any partition is mounted
	IsRaidVolume bool          `json:"isRaidVolume"` // true for md devices (RAID array volumes)
	Segments     []PartSegment `json:"segments"`
}

// partToolSupportedFs are the filesystems offered by the format tool.
// They map to what diskfs.FormatStorageDevice can create.
var partToolSupportedFs = map[string]bool{
	"ext4":  true,
	"ntfs":  true,
	"fat32": true,
}

func partedAvailable() bool {
	return commandExists("parted")
}

// mountPointIsSystem reports whether a mount point belongs to the running OS.
func mountPointIsSystem(mp string) bool {
	return mp == "/" || strings.HasPrefix(mp, "/boot") || mp == "[SWAP]"
}

// raidMemberDevices returns the set of device names (sdX or sdX1) that are
// members of any md array.
func raidMemberDevices() map[string]bool {
	members := map[string]bool{}
	if raidManager == nil {
		return members
	}
	pools, err := raidManager.GetRAIDDevicesFromProcMDStat()
	if err != nil {
		return members
	}
	for _, md := range pools {
		for _, member := range md.Members {
			members[member.Name] = true
		}
	}
	return members
}

// partitionDevPath builds the partition device path for a disk + number,
// handling the pN suffix used by nvme / mmcblk style names.
func partitionDevPath(diskName string, number int) string {
	last := diskName[len(diskName)-1]
	if last >= '0' && last <= '9' {
		return fmt.Sprintf("/dev/%sp%d", diskName, number)
	}
	return fmt.Sprintf("/dev/%s%d", diskName, number)
}

// parseMdstatNames extracts the md device names from /proc/mdstat content.
func parseMdstatNames(content string) []string {
	names := []string{}
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == ":" && strings.HasPrefix(fields[0], "md") {
			names = append(names, fields[0])
		}
	}
	return names
}

// listMdDevices returns the names of all assembled md RAID volumes.
func listMdDevices() []string {
	content, err := os.ReadFile("/proc/mdstat")
	if err != nil {
		return nil
	}
	return parseMdstatNames(string(content))
}

// buildMdLayout builds the layout of one md RAID volume. md devices are
// nested under their member disks in the full lsblk tree, so they are
// queried individually here.
func buildMdLayout(mdName string) (*PartDiskLayout, error) {
	cmd := exec.Command("sudo", "lsblk", "-b", "--json", "/dev/"+mdName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("lsblk failed for /dev/%s", mdName)
	}
	var meta diskfs.StorageDevicesMeta
	if err := json.Unmarshal(output, &meta); err != nil || len(meta.Blockdevices) == 0 {
		return nil, fmt.Errorf("unable to parse lsblk output for /dev/%s", mdName)
	}
	device := meta.Blockdevices[0]

	if mountPointIsSystem(device.Mountpoint) {
		return nil, errors.New("md volume hosts system paths")
	}

	busy := device.Mountpoint != ""
	mountpoints := map[string]string{}
	mountpoints[device.Name] = device.Mountpoint
	for _, child := range device.Children {
		if mountPointIsSystem(child.Mountpoint) {
			return nil, errors.New("md volume hosts system paths")
		}
		if child.Mountpoint != "" {
			busy = true
		}
		mountpoints[child.Name] = child.Mountpoint
	}

	layout := &PartDiskLayout{
		Name:         device.Name,
		Path:         "/dev/" + device.Name,
		SizeByte:     device.Size,
		Busy:         busy,
		IsRaidVolume: true,
		Segments:     []PartSegment{},
	}
	fillPartedLayout(layout, mountpoints)
	return layout, nil
}

// listEditableDisks returns every disk the partition tool may touch,
// with layout information from parted.
func listEditableDisks() ([]*PartDiskLayout, error) {
	storageDevices, err := diskfs.ListAllStorageDevices()
	if err != nil {
		return nil, err
	}
	raidMembers := raidMemberDevices()

	results := []*PartDiskLayout{}
	for _, device := range storageDevices.Blockdevices {
		// md devices (assembled RAID volumes) are editable so the user can
		// format the RAID result disk; their member disks stay hidden
		isMdDevice := strings.HasPrefix(device.Name, "md") && strings.HasPrefix(device.Type, "raid")
		if device.Type != "disk" && !isMdDevice {
			continue
		}
		if !isMdDevice && (strings.HasPrefix(device.Name, "loop") || strings.HasPrefix(device.Name, "zram") || strings.HasPrefix(device.Name, "sr") || strings.HasPrefix(device.Name, "md")) {
			continue
		}

		// Never expose the disk running the OS or disks in a RAID array
		isSystemDisk := mountPointIsSystem(device.Mountpoint)
		isRaidDisk := !isMdDevice && raidMembers[device.Name]
		busy := device.Mountpoint != ""
		mountpoints := map[string]string{} // partition name → mountpoint
		mountpoints[device.Name] = device.Mountpoint
		for _, child := range device.Children {
			if mountPointIsSystem(child.Mountpoint) {
				isSystemDisk = true
			}
			if raidMembers[child.Name] {
				isRaidDisk = true
			}
			if child.Mountpoint != "" {
				busy = true
			}
			mountpoints[child.Name] = child.Mountpoint
		}
		if isSystemDisk || isRaidDisk {
			continue
		}

		layout := &PartDiskLayout{
			Name:         device.Name,
			Path:         "/dev/" + device.Name,
			SizeByte:     device.Size,
			Busy:         busy,
			IsRaidVolume: isMdDevice,
			Segments:     []PartSegment{},
		}
		fillPartedLayout(layout, mountpoints)
		results = append(results, layout)
	}

	// md RAID volumes are nested under their member disks in the lsblk
	// tree, so the top-level loop above never sees them. Append every
	// assembled array so the user can partition / format the RAID result
	// disk (e.g. /dev/md0).
	alreadyListed := map[string]bool{}
	for _, layout := range results {
		alreadyListed[layout.Name] = true
	}
	for _, mdName := range listMdDevices() {
		if alreadyListed[mdName] {
			continue
		}
		layout, err := buildMdLayout(mdName)
		if err != nil {
			log.Println("[PartTool] Skipping " + mdName + ": " + err.Error())
			continue
		}
		results = append(results, layout)
	}

	return results, nil
}

// fillPartedLayout runs `parted print free` on the disk and fills the
// table type, model and segment list of the layout.
func fillPartedLayout(layout *PartDiskLayout, mountpoints map[string]string) {
	cmd := exec.Command("sudo", "parted", "-sm", layout.Path, "unit", "B", "print", "free")
	output, err := cmd.CombinedOutput()
	outStr := string(output)

	if err != nil && strings.Contains(outStr, "unrecognised disk label") {
		// Brand new disk without a partition table: one big free segment
		layout.Table = "none"
		layout.Segments = append(layout.Segments, PartSegment{
			Type:      "free",
			StartByte: 0,
			EndByte:   layout.SizeByte,
			SizeByte:  layout.SizeByte,
		})
		return
	}
	if err != nil {
		log.Println("[PartTool] parted print failed on " + layout.Path + ": " + strings.TrimSpace(outStr))
		layout.Table = "unknown"
		return
	}

	parsePartedOutput(layout, outStr, mountpoints)
}

// parsePartedOutput parses `parted -sm ... unit B print free` machine
// readable output into the layout's table type, model and segments.
func parsePartedOutput(layout *PartDiskLayout, outStr string, mountpoints map[string]string) {
	parseB := func(field string) int64 {
		v, _ := strconv.ParseInt(strings.TrimSuffix(field, "B"), 10, 64)
		return v
	}

	for _, line := range strings.Split(outStr, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(line), ";"))
		if line == "" || line == "BYT" {
			continue
		}
		fields := strings.Split(line, ":")
		if strings.HasPrefix(line, "/dev/") {
			// Disk header: path:size:transport:lsec:psec:table:model:flags
			if len(fields) >= 7 {
				layout.Table = fields[5]
				layout.Model = fields[6]
			}
			continue
		}
		if len(fields) < 5 {
			continue
		}

		if fields[4] == "free" {
			layout.Segments = append(layout.Segments, PartSegment{
				Type:      "free",
				StartByte: parseB(fields[1]),
				EndByte:   parseB(fields[2]),
				SizeByte:  parseB(fields[3]),
			})
			continue
		}

		number, _ := strconv.Atoi(fields[0])
		segment := PartSegment{
			Type:      "partition",
			Number:    number,
			Path:      partitionDevPath(layout.Name, number),
			StartByte: parseB(fields[1]),
			EndByte:   parseB(fields[2]),
			SizeByte:  parseB(fields[3]),
			FsType:    fields[4],
		}
		if layout.Table == "loop" {
			// "loop" means the whole device holds a filesystem directly
			// without a partition table (common for md RAID volumes)
			segment.Number = 0
			segment.Path = layout.Path
		}
		if len(fields) >= 6 {
			segment.Label = fields[5]
		}
		partName := strings.TrimPrefix(segment.Path, "/dev/")
		if mp, ok := mountpoints[partName]; ok && mp != "" {
			segment.MountPoint = mp
			segment.Mounted = true
		}
		layout.Segments = append(layout.Segments, segment)
	}
}

// getEditableDisk re-validates that the given disk may be modified and
// returns its current layout.
func getEditableDisk(diskName string) (*PartDiskLayout, error) {
	diskName = filepath.Base(diskName)
	disks, err := listEditableDisks()
	if err != nil {
		return nil, err
	}
	for _, disk := range disks {
		if disk.Name == diskName {
			return disk, nil
		}
	}
	return nil, errors.New("target disk not found or not editable (system disks and RAID members are protected)")
}

// rereadPartitionTable asks the kernel to reload the partition table.
func rereadPartitionTable(diskPath string) {
	if commandExists("partprobe") {
		exec.Command("sudo", "partprobe", diskPath).Run()
	} else {
		exec.Command("sudo", "blockdev", "--rereadpt", diskPath).Run()
	}
	// Give udev a moment to create the device nodes
	time.Sleep(500 * time.Millisecond)
}

func HandlePartToolCalls() http.Handler {
	return http.StripPrefix("/parttool/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pathParts := strings.Split(r.URL.Path, "/")

		if !partedAvailable() && pathParts[0] != "list" {
			utils.SendErrorResponse(w, "parted is not installed on this host")
			return
		}

		switch pathParts[0] {
		case "list":
			handlePartToolList(w, r)
		case "mklabel":
			handlePartToolMklabel(w, r)
		case "mkpart":
			handlePartToolMkpart(w, r)
		case "rmpart":
			handlePartToolRmpart(w, r)
		case "rmparts":
			handlePartToolRmparts(w, r)
		case "recreate":
			handlePartToolRecreate(w, r)
		case "extend":
			handlePartToolExtend(w, r)
		case "format":
			handlePartToolFormat(w, r)
		default:
			http.Error(w, "Not Found", http.StatusNotFound)
		}
	}))
}

func handlePartToolList(w http.ResponseWriter, r *http.Request) {
	if !partedAvailable() {
		// Frontend renders the install hint from /api/info/deps
		utils.SendErrorResponse(w, "parted is not installed on this host")
		return
	}
	disks, err := listEditableDisks()
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}
	js, _ := json.Marshal(disks)
	utils.SendJSONResponse(w, string(js))
}

// handlePartToolMklabel creates a brand new partition table, destroying
// everything on the disk.
func handlePartToolMklabel(w http.ResponseWriter, r *http.Request) {
	diskName, err := utils.PostPara(r, "disk")
	if err != nil {
		utils.SendErrorResponse(w, "disk not given")
		return
	}
	table, err := utils.PostPara(r, "table")
	if err != nil || (table != "gpt" && table != "msdos") {
		utils.SendErrorResponse(w, "table must be gpt or msdos")
		return
	}

	disk, err := getEditableDisk(diskName)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}
	if disk.Busy {
		utils.SendErrorResponse(w, "disk has mounted partitions, unmount them first")
		return
	}

	cmd := exec.Command("sudo", "parted", "-s", disk.Path, "mklabel", table)
	if output, err := utils.RunAndStream(cmd); err != nil {
		utils.SendErrorResponse(w, "mklabel failed: "+strings.TrimSpace(string(output)))
		return
	}
	rereadPartitionTable(disk.Path)

	log.Println("[PartTool] Created new " + table + " partition table on " + disk.Path)
	utils.SendOK(w)
}

// handlePartToolMkpart creates a partition inside a free segment and
// optionally formats it.
func handlePartToolMkpart(w http.ResponseWriter, r *http.Request) {
	diskName, err := utils.PostPara(r, "disk")
	if err != nil {
		utils.SendErrorResponse(w, "disk not given")
		return
	}
	startStr, err := utils.PostPara(r, "start")
	if err != nil {
		utils.SendErrorResponse(w, "start byte not given")
		return
	}
	endStr, err := utils.PostPara(r, "end")
	if err != nil {
		utils.SendErrorResponse(w, "end byte not given")
		return
	}
	fsType, _ := utils.PostPara(r, "fs") // optional: format after creation

	start, err1 := strconv.ParseInt(startStr, 10, 64)
	end, err2 := strconv.ParseInt(endStr, 10, 64)
	if err1 != nil || err2 != nil || start < 0 || end <= start {
		utils.SendErrorResponse(w, "invalid partition range")
		return
	}
	if fsType != "" && !partToolSupportedFs[fsType] {
		utils.SendErrorResponse(w, "unsupported filesystem type")
		return
	}

	disk, err := getEditableDisk(diskName)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}
	if disk.Table == "none" || disk.Table == "unknown" {
		utils.SendErrorResponse(w, "disk has no partition table, create one first")
		return
	}
	if end > disk.SizeByte {
		utils.SendErrorResponse(w, "partition end exceeds disk size")
		return
	}

	// The requested range must fall inside one free segment
	insideFree := false
	for _, segment := range disk.Segments {
		if segment.Type == "free" && start >= segment.StartByte && end <= segment.EndByte {
			insideFree = true
			break
		}
	}
	if !insideFree {
		utils.SendErrorResponse(w, "requested range overlaps an existing partition")
		return
	}

	// parted wants a filesystem hint for mkpart, it does not create the fs
	fsHint := fsType
	if fsHint == "" {
		fsHint = "ext4"
	}
	cmd := exec.Command("sudo", "parted", "-s", "--align", "optimal", disk.Path,
		"unit", "B", "mkpart", "primary", fsHint, fmt.Sprintf("%dB", start), fmt.Sprintf("%dB", end))
	if output, err := utils.RunAndStream(cmd); err != nil {
		utils.SendErrorResponse(w, "mkpart failed: "+strings.TrimSpace(string(output)))
		return
	}
	rereadPartitionTable(disk.Path)

	// Locate the newly created partition to format it
	if fsType != "" {
		updated, err := getEditableDisk(disk.Name)
		if err != nil {
			utils.SendErrorResponse(w, "partition created but relocating it failed: "+err.Error())
			return
		}
		newPartPath := ""
		for _, segment := range updated.Segments {
			if segment.Type == "partition" && segment.StartByte >= start-(1<<20) && segment.StartByte <= end && segment.EndByte <= end+(1<<20) {
				newPartPath = segment.Path
			}
		}
		if newPartPath == "" {
			utils.SendErrorResponse(w, "partition created but could not be located for formatting")
			return
		}
		if err := diskfs.FormatStorageDevice(fsType, newPartPath); err != nil {
			utils.SendErrorResponse(w, "partition created but format failed: "+err.Error())
			return
		}
		log.Println("[PartTool] Created and formatted " + newPartPath + " as " + fsType)
	} else {
		log.Println("[PartTool] Created new partition on " + disk.Path)
	}

	utils.SendOK(w)
}

// handlePartToolRmpart deletes a partition by number.
func handlePartToolRmpart(w http.ResponseWriter, r *http.Request) {
	diskName, err := utils.PostPara(r, "disk")
	if err != nil {
		utils.SendErrorResponse(w, "disk not given")
		return
	}
	numberStr, err := utils.PostPara(r, "number")
	if err != nil {
		utils.SendErrorResponse(w, "partition number not given")
		return
	}
	number, err := strconv.Atoi(numberStr)
	if err != nil || number < 1 {
		utils.SendErrorResponse(w, "invalid partition number")
		return
	}

	disk, err := getEditableDisk(diskName)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	// The partition must exist and must not be mounted
	found := false
	for _, segment := range disk.Segments {
		if segment.Type == "partition" && segment.Number == number {
			found = true
			if segment.Mounted {
				utils.SendErrorResponse(w, "partition is mounted, unmount it first")
				return
			}
		}
	}
	if !found {
		utils.SendErrorResponse(w, "partition not found on this disk")
		return
	}

	cmd := exec.Command("sudo", "parted", "-s", disk.Path, "rm", strconv.Itoa(number))
	if output, err := utils.RunAndStream(cmd); err != nil {
		utils.SendErrorResponse(w, "delete failed: "+strings.TrimSpace(string(output)))
		return
	}
	rereadPartitionTable(disk.Path)

	log.Println("[PartTool] Deleted partition " + strconv.Itoa(number) + " on " + disk.Path)
	utils.SendOK(w)
}

// handlePartToolRmparts deletes multiple partitions in one call,
// require "disk" and "numbers" (JSON int array).
func handlePartToolRmparts(w http.ResponseWriter, r *http.Request) {
	diskName, err := utils.PostPara(r, "disk")
	if err != nil {
		utils.SendErrorResponse(w, "disk not given")
		return
	}
	numbersJSON, err := utils.PostPara(r, "numbers")
	if err != nil {
		utils.SendErrorResponse(w, "partition numbers not given")
		return
	}
	numbers := []int{}
	if err := json.Unmarshal([]byte(numbersJSON), &numbers); err != nil || len(numbers) == 0 {
		utils.SendErrorResponse(w, "unable to parse partition numbers")
		return
	}

	disk, err := getEditableDisk(diskName)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	// Validate every requested partition before touching anything
	for _, number := range numbers {
		found := false
		for _, segment := range disk.Segments {
			if segment.Type == "partition" && segment.Number == number {
				found = true
				if segment.Mounted {
					utils.SendErrorResponse(w, fmt.Sprintf("partition %d is mounted, unmount it first", number))
					return
				}
			}
		}
		if !found {
			utils.SendErrorResponse(w, fmt.Sprintf("partition %d not found on this disk", number))
			return
		}
	}

	// Delete highest number first so msdos logical renumbering cannot
	// shift the remaining targets
	sort.Sort(sort.Reverse(sort.IntSlice(numbers)))
	for _, number := range numbers {
		cmd := exec.Command("sudo", "parted", "-s", disk.Path, "rm", strconv.Itoa(number))
		if output, err := utils.RunAndStream(cmd); err != nil {
			rereadPartitionTable(disk.Path)
			utils.SendErrorResponse(w, fmt.Sprintf("delete of partition %d failed: %s", number, strings.TrimSpace(string(output))))
			return
		}
	}
	rereadPartitionTable(disk.Path)

	log.Printf("[PartTool] Deleted %d partition(s) on %s", len(numbers), disk.Path)
	utils.SendOK(w)
}

// recreatePartSpec is one requested partition in a disk recreate call.
type recreatePartSpec struct {
	SizeMB int64  `json:"sizeMB"` // 0 = use all remaining space (last entry only)
	Fs     string `json:"fs"`     // ext4 / ntfs / fat32 / "" (leave unformatted)
}

type recreateCreatedPart struct {
	start int64
	end   int64
	fs    string
}

func recreatePartEnd(start int64, sizeMB int64, diskSize int64, alignment int64) int64 {
	if sizeMB == 0 {
		return diskSize - alignment - 1
	}
	return start + sizeMB*alignment - 1
}

func findCreatedRecreateSegment(layout *PartDiskLayout, requestedStart int64, requestedEnd int64, seen map[int]bool) *PartSegment {
	var best *PartSegment
	for i := range layout.Segments {
		segment := &layout.Segments[i]
		if segment.Type != "partition" || seen[segment.Number] {
			continue
		}
		if segment.EndByte < requestedStart || segment.StartByte > requestedEnd {
			continue
		}
		if best == nil || segment.StartByte < best.StartByte {
			best = segment
		}
	}
	return best
}

// handlePartToolRecreate wipes the disk with a fresh partition table and
// creates multiple partitions (optionally formatted) in one go.
// Require "disk", "table" (gpt|msdos) and "parts" (JSON recreatePartSpec array).
func handlePartToolRecreate(w http.ResponseWriter, r *http.Request) {
	diskName, err := utils.PostPara(r, "disk")
	if err != nil {
		utils.SendErrorResponse(w, "disk not given")
		return
	}
	table, err := utils.PostPara(r, "table")
	if err != nil || (table != "gpt" && table != "msdos") {
		utils.SendErrorResponse(w, "table must be gpt or msdos")
		return
	}
	partsJSON, err := utils.PostPara(r, "parts")
	if err != nil {
		utils.SendErrorResponse(w, "partition list not given")
		return
	}
	parts := []recreatePartSpec{}
	if err := json.Unmarshal([]byte(partsJSON), &parts); err != nil || len(parts) == 0 {
		utils.SendErrorResponse(w, "unable to parse partition list")
		return
	}
	if len(parts) > 128 {
		utils.SendErrorResponse(w, "too many partitions")
		return
	}
	if table == "msdos" && len(parts) > 4 {
		utils.SendErrorResponse(w, "msdos tables support at most 4 primary partitions, use GPT for more")
		return
	}

	disk, err := getEditableDisk(diskName)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}
	if disk.Busy {
		utils.SendErrorResponse(w, "disk has mounted partitions, unmount them first")
		return
	}

	// Validate the layout fits: partitions start at 1MiB, sizeMB 0 means
	// "rest of the disk" and is only allowed on the last entry
	const alignment = int64(1 << 20)
	cursor := alignment
	for i, part := range parts {
		if part.Fs != "" && !partToolSupportedFs[part.Fs] {
			utils.SendErrorResponse(w, "unsupported filesystem type: "+part.Fs)
			return
		}
		if part.SizeMB == 0 && i != len(parts)-1 {
			utils.SendErrorResponse(w, "only the last partition may use the remaining space")
			return
		}
		if part.SizeMB < 0 {
			utils.SendErrorResponse(w, "invalid partition size")
			return
		}
		if part.SizeMB > 0 {
			cursor += part.SizeMB * alignment
		}
	}
	// Leave 1MiB tail room for the GPT backup header
	if cursor > disk.SizeByte-alignment {
		utils.SendErrorResponse(w, "requested layout exceeds the disk size")
		return
	}

	// Wipe and create the new table
	cmd := exec.Command("sudo", "parted", "-s", disk.Path, "mklabel", table)
	if output, err := utils.RunAndStream(cmd); err != nil {
		utils.SendErrorResponse(w, "mklabel failed: "+strings.TrimSpace(string(output)))
		return
	}

	// Create every partition. parted may adjust the exact boundaries for
	// alignment, so use the real partition end as the next start.
	created := []recreateCreatedPart{}
	seenNumbers := map[int]bool{}
	cursor = alignment
	for i, part := range parts {
		start := cursor
		end := recreatePartEnd(start, part.SizeMB, disk.SizeByte, alignment)

		fsHint := part.Fs
		if fsHint == "" {
			fsHint = "ext4"
		}
		cmd := exec.Command("sudo", "parted", "-s", "--align", "optimal", disk.Path,
			"unit", "B", "mkpart", "primary", fsHint, fmt.Sprintf("%dB", start), fmt.Sprintf("%dB", end))
		if output, err := utils.RunAndStream(cmd); err != nil {
			rereadPartitionTable(disk.Path)
			utils.SendErrorResponse(w, fmt.Sprintf("creating partition %d failed: %s", i+1, strings.TrimSpace(string(output))))
			return
		}
		rereadPartitionTable(disk.Path)
		updated, err := getEditableDisk(disk.Name)
		if err != nil {
			utils.SendErrorResponse(w, fmt.Sprintf("partition %d created but layout reload failed: %s", i+1, err.Error()))
			return
		}
		createdPart := findCreatedRecreateSegment(updated, start, end, seenNumbers)
		if createdPart == nil {
			utils.SendErrorResponse(w, fmt.Sprintf("partition %d created but could not be resolved after alignment", i+1))
			return
		}
		seenNumbers[createdPart.Number] = true
		created = append(created, recreateCreatedPart{start: createdPart.StartByte, end: createdPart.EndByte, fs: part.Fs})
		cursor = createdPart.EndByte + 1
	}

	// Format the new partitions. Re-read the layout to resolve the real
	// partition numbers / paths (parted may renumber msdos entries).
	updated, err := getEditableDisk(disk.Name)
	if err != nil {
		utils.SendErrorResponse(w, "partitions created but layout reload failed: "+err.Error())
		return
	}
	formatErrors := []string{}
	for _, want := range created {
		if want.fs == "" {
			continue
		}
		targetPath := ""
		for _, segment := range updated.Segments {
			if segment.Type == "partition" && segment.StartByte >= want.start-alignment && segment.StartByte <= want.start+alignment {
				targetPath = segment.Path
				break
			}
		}
		if targetPath == "" {
			formatErrors = append(formatErrors, fmt.Sprintf("partition at offset %d not found after creation", want.start))
			continue
		}
		if err := diskfs.FormatStorageDevice(want.fs, targetPath); err != nil {
			formatErrors = append(formatErrors, targetPath+": "+err.Error())
		}
	}
	if len(formatErrors) > 0 {
		utils.SendErrorResponse(w, "partitions created but formatting failed: "+strings.Join(formatErrors, "; "))
		return
	}

	log.Printf("[PartTool] Recreated %s with %s table and %d partition(s)", disk.Path, table, len(parts))
	utils.SendOK(w)
}

// growFilesystem enlarges the filesystem on a partition to fill the (just
// extended) partition. Filesystem-specific userspace tools are required.
func growFilesystem(fsType string, partPath string) error {
	switch fsType {
	case "ext2", "ext3", "ext4":
		// resize2fs on an unmounted fs requires a clean fsck pass first
		checkCmd := exec.Command("sudo", "e2fsck", "-f", "-y", partPath)
		if output, err := utils.RunAndStream(checkCmd); err != nil {
			return errors.New("filesystem check failed: " + strings.TrimSpace(string(output)))
		}
		cmd := exec.Command("sudo", "resize2fs", partPath)
		if output, err := utils.RunAndStream(cmd); err != nil {
			return errors.New("resize2fs failed: " + strings.TrimSpace(string(output)))
		}
		return nil
	case "ntfs":
		if !commandExists("ntfsresize") {
			return errors.New("resizing NTFS requires the ntfs-3g package")
		}
		// Without a size argument ntfsresize grows to the partition size,
		// -f skips the interactive confirmation
		cmd := exec.Command("sudo", "ntfsresize", "-f", partPath)
		if output, err := utils.RunAndStream(cmd); err != nil {
			return errors.New("ntfsresize failed: " + strings.TrimSpace(string(output)))
		}
		return nil
	case "fat32", "fat16", "vfat":
		if !commandExists("fatresize") {
			return errors.New("resizing FAT filesystems requires the fatresize package (sudo apt-get install -y fatresize)")
		}
		cmd := exec.Command("sudo", "fatresize", "-s", "max", partPath)
		if output, err := utils.RunAndStream(cmd); err != nil {
			return errors.New("fatresize failed: " + strings.TrimSpace(string(output)))
		}
		return nil
	case "":
		// No filesystem, only the partition boundary moves
		return nil
	default:
		return errors.New("growing " + fsType + " filesystems is not supported, the partition was extended but the filesystem still has its old size")
	}
}

// handlePartToolExtend grows a partition into the unused space directly
// after it (no merging with other partitions), then grows the filesystem.
// Require "disk", "number" and "end" (new end byte).
func handlePartToolExtend(w http.ResponseWriter, r *http.Request) {
	diskName, err := utils.PostPara(r, "disk")
	if err != nil {
		utils.SendErrorResponse(w, "disk not given")
		return
	}
	numberStr, err := utils.PostPara(r, "number")
	if err != nil {
		utils.SendErrorResponse(w, "partition number not given")
		return
	}
	number, err := strconv.Atoi(numberStr)
	if err != nil || number < 1 {
		utils.SendErrorResponse(w, "invalid partition number")
		return
	}
	endStr, err := utils.PostPara(r, "end")
	if err != nil {
		utils.SendErrorResponse(w, "new end byte not given")
		return
	}
	newEnd, err := strconv.ParseInt(endStr, 10, 64)
	if err != nil || newEnd <= 0 {
		utils.SendErrorResponse(w, "invalid new end")
		return
	}

	disk, err := getEditableDisk(diskName)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	// Locate the partition and the free segment directly after it
	var target *PartSegment
	var following *PartSegment
	for i := range disk.Segments {
		if disk.Segments[i].Type == "partition" && disk.Segments[i].Number == number {
			target = &disk.Segments[i]
			if i+1 < len(disk.Segments) {
				following = &disk.Segments[i+1]
			}
			break
		}
	}
	if target == nil {
		utils.SendErrorResponse(w, "partition not found on this disk")
		return
	}
	if target.Mounted {
		utils.SendErrorResponse(w, "partition is mounted, unmount it first")
		return
	}
	if newEnd <= target.EndByte {
		utils.SendErrorResponse(w, "new end must be larger than the current partition end (shrinking is not supported)")
		return
	}
	if following == nil || following.Type != "free" {
		utils.SendErrorResponse(w, "there is no unused space directly after this partition")
		return
	}
	if newEnd > following.EndByte {
		utils.SendErrorResponse(w, "new end exceeds the unused space after this partition")
		return
	}

	// Grow the partition boundary
	cmd := exec.Command("sudo", "parted", "-s", disk.Path, "unit", "B",
		"resizepart", strconv.Itoa(number), fmt.Sprintf("%dB", newEnd))
	if output, err := utils.RunAndStream(cmd); err != nil {
		utils.SendErrorResponse(w, "resizepart failed: "+strings.TrimSpace(string(output)))
		return
	}
	rereadPartitionTable(disk.Path)

	// Grow the filesystem to fill the new boundary
	if err := growFilesystem(target.FsType, target.Path); err != nil {
		utils.SendErrorResponse(w, "partition extended but filesystem grow failed: "+err.Error())
		return
	}

	log.Printf("[PartTool] Extended partition %s to end at byte %d", target.Path, newEnd)
	utils.SendOK(w)
}

// handlePartToolFormat formats an existing partition.
func handlePartToolFormat(w http.ResponseWriter, r *http.Request) {
	partName, err := utils.PostPara(r, "partition")
	if err != nil {
		utils.SendErrorResponse(w, "partition not given")
		return
	}
	fsType, err := utils.PostPara(r, "fs")
	if err != nil || !partToolSupportedFs[fsType] {
		utils.SendErrorResponse(w, "unsupported filesystem type")
		return
	}

	partName = filepath.Base(partName)

	// Whole-device format: the target itself is an editable disk. This is
	// how RAID volumes (/dev/md0) get their filesystem, they usually carry
	// no partition table.
	if wholeDisk, err := getEditableDisk(partName); err == nil {
		if wholeDisk.Busy {
			utils.SendErrorResponse(w, "device has mounted filesystems, unmount them first")
			return
		}
		if err := diskfs.FormatStorageDevice(fsType, wholeDisk.Path); err != nil {
			utils.SendErrorResponse(w, err.Error())
			return
		}
		log.Println("[PartTool] Formatted whole device " + wholeDisk.Path + " as " + fsType)
		utils.SendOK(w)
		return
	}

	// Derive the parent disk name by trimming the partition suffix
	diskName := strings.TrimRight(partName, "0123456789")
	diskName = strings.TrimSuffix(diskName, "p")

	disk, err := getEditableDisk(diskName)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	partPath := "/dev/" + partName
	found := false
	for _, segment := range disk.Segments {
		if segment.Type == "partition" && segment.Path == partPath {
			found = true
			if segment.Mounted {
				utils.SendErrorResponse(w, "partition is mounted, unmount it first")
				return
			}
		}
	}
	if !found {
		utils.SendErrorResponse(w, "partition not found")
		return
	}

	if err := diskfs.FormatStorageDevice(fsType, partPath); err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	log.Println("[PartTool] Formatted " + partPath + " as " + fsType)
	utils.SendOK(w)
}
