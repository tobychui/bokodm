package fdisk

import (
	"bytes"
	"os/exec"
	"strings"
)

type DiskInfo struct {
	Name       string //e.g. /dev/sda
	Model      string //e.g. Samsung SSD 860 EVO 1TB
	DiskLabel  string //e.g. gpt
	Identifier string //e.g. 0x12345678
}

func GetDiskModelAndIdentifier(disk string) (*DiskInfo, error) {
	//Make sure there is /dev/ prefix
	if !strings.HasPrefix(disk, "/dev/") {
		disk = "/dev/" + disk
	}
	//Make sure there is no trailing slash
	disk = strings.TrimSuffix(disk, "/")

	cmd := exec.Command("fdisk", "-l", disk)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(out.String(), "\n")

	//Only extracting the upper section of disk info
	var info DiskInfo = DiskInfo{
		Name: disk,
	}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Disk model:") {
			info.Model = strings.TrimPrefix(line, "Disk model: ")
		} else if strings.HasPrefix(line, "Disklabel type:") {
			info.DiskLabel = strings.TrimPrefix(line, "Disklabel type: ")
		} else if strings.HasPrefix(line, "Disk identifier:") {
			info.Identifier = strings.TrimPrefix(line, "Disk identifier: ")
		}
	}

	return &info, nil
}
