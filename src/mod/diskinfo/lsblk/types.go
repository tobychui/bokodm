package lsblk

// BlockDevice represents a block device and its attributes.
type BlockDevice struct {
	Name       string        `json:"name"`                 // e.g. sda (Linux) or disk0 (macOS)
	Size       int64         `json:"size"`                 // Size in bytes
	Type       string        `json:"type"`                 // "disk" or "part"
	MountPoint string        `json:"mountpoint,omitempty"` // Mount point if mounted
	Children   []BlockDevice `json:"children,omitempty"`   // Partitions
}
