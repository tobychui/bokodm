//go:build linux || darwin
// +build linux darwin

package df

import (
	"bytes"
	"errors"
	"os/exec"
	"strconv"
	"strings"
)

func getDiskUsageByPath(path string) (*DiskInfo, error) {
	if !strings.HasPrefix(path, "/dev/") {
		path = "/dev/" + path
	}
	path = strings.TrimSuffix(path, "/")

	diskUsages, err := getDiskUsage()
	if err != nil {
		return nil, err
	}

	for _, diskInfo := range diskUsages {
		if strings.HasPrefix(diskInfo.DevicePath, path) {
			return &diskInfo, nil
		}
	}
	return nil, errors.New("disk usage not found for path: " + path)
}

// getDiskUsage runs `df -Pk` which produces a POSIX-compatible 6-column output
// on both Linux and macOS, with sizes in 1 KiB blocks.
func getDiskUsage() ([]DiskInfo, error) {
	cmd := exec.Command("df", "-Pk")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	lines := strings.Split(out.String(), "\n")
	if len(lines) < 2 {
		return nil, nil
	}

	var diskInfos []DiskInfo
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}

		usePercent, err := strconv.Atoi(strings.TrimSuffix(fields[4], "%"))
		if err != nil {
			continue
		}

		blocks, _ := strconv.ParseInt(fields[1], 10, 64)
		used, _ := strconv.ParseInt(fields[2], 10, 64)
		available, _ := strconv.ParseInt(fields[3], 10, 64)

		diskInfos = append(diskInfos, DiskInfo{
			DevicePath: fields[0],
			Blocks:     blocks,
			Used:       used * 1024,      // 1K-blocks → bytes
			Available:  available * 1024, // 1K-blocks → bytes
			UsePercent: usePercent,
			MountedOn:  fields[5],
		})
	}
	return diskInfos, nil
}
