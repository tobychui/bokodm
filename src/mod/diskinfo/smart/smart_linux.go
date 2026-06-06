//go:build linux
// +build linux

package smart

import (
	"errors"
	"strings"

	"imuslab.com/bokofs/bokofsd/mod/diskinfo"
)

// getDiskType uses Linux device naming conventions to identify the disk type.
// /dev/nvme* → NVMe, /dev/sd* → SATA.
func getDiskType(disk string) (DiskType, error) {
	if !strings.HasPrefix(disk, "/dev/") {
		disk = "/dev/" + disk
	}
	if !diskinfo.DevicePathIsValidDisk(disk) {
		return DiskType_Unknown, errors.New("disk is not a valid disk")
	}

	switch {
	case strings.HasPrefix(disk, "/dev/nvme"):
		return DiskType_NVMe, nil
	case strings.HasPrefix(disk, "/dev/sd"):
		return DiskType_SATA, nil
	}
	return DiskType_Unknown, errors.New("disk is not NVMe or SATA")
}
