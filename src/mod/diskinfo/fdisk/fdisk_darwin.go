//go:build darwin
// +build darwin

package fdisk

import (
	"fmt"
	"os/exec"
	"strings"
)

func getDiskModelAndIdentifier(disk string) (*DiskInfo, error) {
	if !strings.HasPrefix(disk, "/dev/") {
		disk = "/dev/" + disk
	}
	disk = strings.TrimSuffix(disk, "/")

	out, err := exec.Command("diskutil", "info", disk).Output()
	if err != nil {
		return nil, fmt.Errorf("diskutil info failed for %s: %w", disk, err)
	}

	info := &DiskInfo{Name: disk}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Partition Map Type:"):
			info.DiskLabel = strings.TrimSpace(strings.TrimPrefix(line, "Partition Map Type:"))
		case strings.HasPrefix(line, "Disk / Partition UUID:"):
			info.Identifier = strings.TrimSpace(strings.TrimPrefix(line, "Disk / Partition UUID:"))
		case strings.HasPrefix(line, "Device / Media Name:"):
			info.Model = strings.TrimSpace(strings.TrimPrefix(line, "Device / Media Name:"))
		case strings.HasPrefix(line, "Device/Media Name:"):
			info.Model = strings.TrimSpace(strings.TrimPrefix(line, "Device/Media Name:"))
		}
	}
	return info, nil
}
