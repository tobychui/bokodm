//go:build linux
// +build linux

package fdisk

import (
	"bytes"
	"os/exec"
	"strings"
)

func getDiskModelAndIdentifier(disk string) (*DiskInfo, error) {
	if !strings.HasPrefix(disk, "/dev/") {
		disk = "/dev/" + disk
	}
	disk = strings.TrimSuffix(disk, "/")

	cmd := exec.Command("sudo", "fdisk", "-l", disk)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	info := &DiskInfo{Name: disk}
	for _, line := range strings.Split(out.String(), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Disk model:"):
			info.Model = strings.TrimPrefix(line, "Disk model: ")
		case strings.HasPrefix(line, "Disklabel type:"):
			info.DiskLabel = strings.TrimPrefix(line, "Disklabel type: ")
		case strings.HasPrefix(line, "Disk identifier:"):
			info.Identifier = strings.TrimPrefix(line, "Disk identifier: ")
		}
	}
	return info, nil
}
