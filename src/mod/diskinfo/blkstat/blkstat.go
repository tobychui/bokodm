package blkstat

/*
	blkstat.go

	Public API for disk I/O statistics and bus-position queries.
	Platform-specific implementations live in blkstat_linux.go / blkstat_darwin.go.
	blkstat_other.go provides stubs for unsupported platforms.
*/

// GetBlockStat returns accumulated I/O statistics for the given block device.
// Pass the BSD device name without the /dev/ prefix (e.g. "sda" or "disk0").
func GetBlockStat(blockName string) (*BlockStat, error) {
	return getBlockStat(blockName)
}

// GetInstalledBus returns the physical bus connection information for the
// given block device (PCIe/SATA/USB/NVMe/Thunderbolt).
func GetInstalledBus(blockName string) (*InstallPosition, error) {
	return getInstalledBus(blockName)
}
