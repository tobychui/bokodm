//go:build !linux
// +build !linux

package smart

import (
	"errors"
	"os/exec"
	"strings"

	"imuslab.com/bokofs/bokofsd/mod/diskinfo"
)

// getDiskType inspects `smartctl -i` output to determine disk type.
// This is used on macOS, Windows, and other platforms where the device-name
// prefix cannot be used (Linux /dev/nvme* / /dev/sd* convention).
func getDiskType(disk string) (DiskType, error) {
	if !strings.HasPrefix(disk, "/dev/") {
		disk = "/dev/" + disk
	}
	if !diskinfo.DevicePathIsValidDisk(disk) {
		return DiskType_Unknown, errors.New("disk is not a valid disk")
	}

	output, err := exec.Command("smartctl", "-i", disk).Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 4 {
			// Exit 4 = SMART not supported; still parse partial output
		} else {
			return DiskType_Unknown, err
		}
	}

	outStr := string(output)
	switch {
	case strings.Contains(outStr, "NVM Commands") ||
		strings.Contains(outStr, "NVMe Version") ||
		strings.Contains(outStr, "NVM Express"):
		return DiskType_NVMe, nil
	case strings.Contains(outStr, "SATA Version") ||
		strings.Contains(outStr, "ATA Standard") ||
		strings.Contains(outStr, "Serial ATA"):
		return DiskType_SATA, nil
	}
	return DiskType_Unknown, errors.New("unable to determine disk type from smartctl output")
}
