package fdisk

/*
	fdisk.go

	Public API for disk model and partition-table label retrieval.
	fdisk_linux.go   — uses `fdisk -l` (Linux)
	fdisk_darwin.go  — uses `diskutil info` (macOS)
	fdisk_other.go   — stub for unsupported platforms
*/

// GetDiskModelAndIdentifier returns the disk model, partition-table label type
// (e.g. "gpt"), and a platform-specific identifier for the given disk device.
// Pass either a full path ("/dev/sda") or just the device name ("sda").
func GetDiskModelAndIdentifier(disk string) (*DiskInfo, error) {
	return getDiskModelAndIdentifier(disk)
}
