package blkid

// BlockDevice holds partition identification info returned by blkid (Linux)
// or diskutil info (macOS).
type BlockDevice struct {
	Device    string // Device path, e.g. /dev/sda1 or /dev/disk0s1
	UUID      string // Filesystem UUID
	BlockSize int    // Block size in bytes
	Type      string // Filesystem type, e.g. ext4, apfs
	PartUUID  string // Partition UUID
	PartLabel string // Partition label / volume name
}
