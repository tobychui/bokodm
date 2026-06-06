package fdisk

type DiskInfo struct {
	Name       string // e.g. /dev/sda (Linux) or /dev/disk0 (macOS)
	Model      string // e.g. Samsung SSD 860 EVO 1TB
	DiskLabel  string // e.g. gpt
	Identifier string // e.g. 0x12345678
}
