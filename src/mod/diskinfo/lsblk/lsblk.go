package lsblk

/*
	lsblk.go

	Public API for block device enumeration.
	lsblk_linux.go   — uses `lsblk -J` (Linux)
	lsblk_darwin.go  — uses `diskutil list -plist` (macOS)
	lsblk_other.go   — stub for unsupported platforms
*/

// GetLSBLKOutput returns all block devices (disks and their partitions) visible
// to the operating system.
func GetLSBLKOutput() ([]BlockDevice, error) {
	return getLSBLKOutput()
}

// GetBlockDeviceInfoFromDevicePath returns the BlockDevice entry for the given
// device, which may be a whole disk (e.g. "sda" / "disk0") or a partition
// (e.g. "sda1" / "disk0s1"). The /dev/ prefix is stripped automatically.
func GetBlockDeviceInfoFromDevicePath(devname string) (*BlockDevice, error) {
	return getBlockDeviceInfoFromDevicePath(devname)
}
