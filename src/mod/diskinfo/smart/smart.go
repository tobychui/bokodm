package smart

/*
	smart.go

	Public API for SMART disk health monitoring via smartctl.
	smart_impl.go    — shared implementation (all platforms with smartctl)
	smart_linux.go   — Linux disk-type detection (uses /dev/nvme*, /dev/sd*)
	smart_other.go   — non-Linux disk-type detection (inspects smartctl -i output)
*/

// GetDiskType returns whether the disk is NVMe, SATA, or unknown.
func GetDiskType(disk string) (DiskType, error) {
	return getDiskType(disk)
}

// GetNVMEInfo retrieves NVMe controller and namespace info via `smartctl -i`.
func GetNVMEInfo(disk string) (*NVMEInfo, error) {
	return getNVMEInfo(disk)
}

// GetSATAInfo retrieves SATA disk identity info via `smartctl -i`.
func GetSATAInfo(disk string) (*SATADiskInfo, error) {
	return getSATAInfo(disk)
}

// SetSMARTEnableOnDisk enables or disables SMART on the specified disk.
func SetSMARTEnableOnDisk(disk string, isEnabled bool) error {
	return setSmartEnable(disk, isEnabled)
}

// GetDiskSMARTCheck runs a SMART health self-assessment and returns the result.
func GetDiskSMARTCheck(diskname string) (*SMARTTestResult, error) {
	return getDiskSmartCheck(diskname)
}

// GetDiskSMARTHealthSummary returns a comprehensive health summary including
// power-on hours, reallocated sectors, wear levelling, and more.
func GetDiskSMARTHealthSummary(diskname string) (*DriveHealthInfo, error) {
	return getDiskSmartHealthSummary(diskname)
}
