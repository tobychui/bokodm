package df

/*
	df.go

	Public API for disk usage statistics.
	df_unix.go   — uses `df -Pk` (Linux and macOS)
	df_other.go  — stub for unsupported platforms
*/

// DiskInfo holds usage statistics for a single mounted filesystem.
type DiskInfo struct {
	DevicePath string
	Blocks     int64
	Used       int64
	Available  int64
	UsePercent int
	MountedOn  string
}

// GetDiskUsageByPath returns disk-usage info for the filesystem that contains
// the given device path (e.g. "/dev/sda1" or "sda1").
func GetDiskUsageByPath(path string) (*DiskInfo, error) {
	return getDiskUsageByPath(path)
}

// GetDiskUsage returns usage info for every mounted filesystem.
func GetDiskUsage() ([]DiskInfo, error) {
	return getDiskUsage()
}
