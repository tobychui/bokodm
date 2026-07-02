package blkstat

// BlockStat holds accumulated disk I/O statistics.
// On Linux these come from /sys/block/<name>/stat; on macOS from gopsutil.
// Fields not available on macOS are left at zero.
type BlockStat struct {
	ReadIOs      uint64
	ReadMerges   uint64
	ReadSectors  uint64
	ReadTicks    uint64
	WriteIOs     uint64
	WriteMerges  uint64
	WriteSectors uint64
	WriteTicks   uint64
	InFlight     uint64
	IoTicks      uint64
	TimeInQueue  uint64
}

// InstallPosition describes the physical bus location of a block device.
type InstallPosition struct {
	PCIEBusAddress string // PCIe bus address or connection type (e.g. "Thunderbolt")
	SATAPort       string // SATA port
	USBPort        string // USB port
	NVMESlot       string // NVMe slot
}
