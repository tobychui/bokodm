package blkid

/*
Package blkid provides functions to retrieve block device information
Usually this will only return partitions info
*/

import (
	"bufio"
	"errors"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

type BlockDevice struct {
	Device    string // Device name (e.g., /dev/sda1)
	UUID      string // UUID of the device
	BlockSize int    // Block size in bytes
	Type      string // Type of the device (e.g., ext4, ntfs)
	PartUUID  string // Partition UUID
	PartLabel string // Partition label
}

// GetBlockDevices retrieves block devices using the `blkid` command.
func GetPartitionIdInfo() ([]BlockDevice, error) {
	cmd := exec.Command("blkid")

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	devices := []BlockDevice{}
	re := regexp.MustCompile(`(\S+):\s+(.*)`)

	for scanner.Scan() {
		line := scanner.Text()
		matches := re.FindStringSubmatch(line)
		if len(matches) != 3 {
			continue
		}

		device := matches[1]
		attributes := matches[2]
		deviceInfo := BlockDevice{Device: device}

		for _, attr := range strings.Split(attributes, " ") {
			kv := strings.SplitN(attr, "=", 2)
			if len(kv) != 2 {
				continue
			}
			key := kv[0]
			value := strings.Trim(kv[1], `"`)

			switch key {
			case "UUID":
				deviceInfo.UUID = value
			case "BLOCK_SIZE":
				// Convert block size to int if possible
				blockSize, err := strconv.Atoi(value)
				if err == nil {
					deviceInfo.BlockSize = blockSize
				} else {
					deviceInfo.BlockSize = 0
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

// GetBlockDeviceIDFromDevicePath retrieves block device information for a given device path.
func GetPartitionIDFromDevicePath(devpath string) (*BlockDevice, error) {
	devpath = strings.TrimPrefix(devpath, "/dev/")
	if strings.Contains(devpath, "/") {
		return nil, errors.New("invalid device path")
	}

	devpath = "/dev/" + devpath

	devices, err := GetPartitionIdInfo()
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
