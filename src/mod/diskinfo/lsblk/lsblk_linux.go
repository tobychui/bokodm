//go:build linux
// +build linux

package lsblk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

func getLSBLKOutput() ([]BlockDevice, error) {
	cmd := exec.Command("lsblk", "-o", "NAME,SIZE,TYPE,MOUNTPOINT", "-b", "-J")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return parseLSBLKJSON(out.String())
}

func getBlockDeviceInfoFromDevicePath(devname string) (*BlockDevice, error) {
	devname = strings.TrimPrefix(devname, "/dev/")
	if strings.Contains(devname, "/") {
		return nil, fmt.Errorf("invalid device name: %s", devname)
	}

	devices, err := getLSBLKOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to get block device info: %w", err)
	}

	for _, device := range devices {
		if device.Name == devname {
			return &device, nil
		}
		for _, child := range device.Children {
			if child.Name == devname {
				return &child, nil
			}
		}
	}
	return nil, fmt.Errorf("device %s not found", devname)
}

func parseLSBLKJSON(output string) ([]BlockDevice, error) {
	var result struct {
		BlockDevices []BlockDevice `json:"blockdevices"`
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		return nil, fmt.Errorf("failed to parse lsblk JSON output: %w", err)
	}
	return result.BlockDevices, nil
}
