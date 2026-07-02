//go:build linux
// +build linux

package blkid

import (
	"bufio"
	"errors"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

func getPartitionIdInfo() ([]BlockDevice, error) {
	cmd := exec.Command("id", "-u")
	userIDOutput, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var blkidCmd *exec.Cmd
	if strings.TrimSpace(string(userIDOutput)) == "0" {
		blkidCmd = exec.Command("blkid")
	} else {
		blkidCmd = exec.Command("sudo", "blkid")
	}

	output, err := blkidCmd.Output()
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	var devices []BlockDevice
	re := regexp.MustCompile(`(\S+):\s+(.*)`)

	for scanner.Scan() {
		line := scanner.Text()
		matches := re.FindStringSubmatch(line)
		if len(matches) != 3 {
			continue
		}

		deviceInfo := BlockDevice{Device: matches[1]}
		for _, attr := range strings.Split(matches[2], " ") {
			kv := strings.SplitN(attr, "=", 2)
			if len(kv) != 2 {
				continue
			}
			value := strings.Trim(kv[1], `"`)
			switch kv[0] {
			case "UUID":
				deviceInfo.UUID = value
			case "BLOCK_SIZE":
				if bs, err := strconv.Atoi(value); err == nil {
					deviceInfo.BlockSize = bs
				}
			case "TYPE":
				deviceInfo.Type = value
			case "PARTUUID":
				deviceInfo.PartUUID = value
			case "PARTLABEL":
				deviceInfo.PartLabel = value
			}
		}
		devices = append(devices, deviceInfo)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return devices, nil
}

func getPartitionIDFromDevicePath(devpath string) (*BlockDevice, error) {
	devpath = strings.TrimPrefix(devpath, "/dev/")
	if strings.Contains(devpath, "/") {
		return nil, errors.New("invalid device path")
	}
	devpath = "/dev/" + devpath

	devices, err := getPartitionIdInfo()
	if err != nil {
		return nil, err
	}

	for _, device := range devices {
		if device.Device == devpath {
			return &device, nil
		}
	}
	return nil, errors.New("device not found")
}
