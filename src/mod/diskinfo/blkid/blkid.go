package blkid

/*
	blkid.go

	Public API for partition identification (UUID, filesystem type, block size).
	blkid_linux.go   — wraps the `blkid` command (Linux)
	blkid_darwin.go  — wraps `diskutil info -plist` (macOS)
	blkid_other.go   — stub for unsupported platforms
*/

// GetPartitionIdInfo returns identification info for all partitions visible
// to the operating system.
func GetPartitionIdInfo() ([]BlockDevice, error) {
	return getPartitionIdInfo()
}

// GetPartitionIDFromDevicePath returns partition identification info for the
// given device path (e.g. "/dev/sda1" or "/dev/disk0s1").
func GetPartitionIDFromDevicePath(devpath string) (*BlockDevice, error) {
	return getPartitionIDFromDevicePath(devpath)
}
