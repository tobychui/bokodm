//go:build linux
// +build linux

package blkstat

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// getBlockStat reads /sys/block/<blockName>/stat for accumulated I/O counters.
func getBlockStat(blockName string) (*BlockStat, error) {
	blockName = strings.TrimPrefix(blockName, "/dev/")
	statPath := fmt.Sprintf("/sys/block/%s/stat", blockName)
	data, err := os.ReadFile(statPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read stat file: %w", err)
	}

	fields := strings.Fields(string(data))
	if len(fields) < 11 {
		return nil, fmt.Errorf("unexpected stat file format")
	}

	values := make([]uint64, 11)
	for i := 0; i < 11; i++ {
		values[i], err = strconv.ParseUint(fields[i], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse stat value: %w", err)
		}
	}

	return &BlockStat{
		ReadIOs:      values[0],
		ReadMerges:   values[1],
		ReadSectors:  values[2],
		ReadTicks:    values[3],
		WriteIOs:     values[4],
		WriteMerges:  values[5],
		WriteSectors: values[6],
		WriteTicks:   values[7],
		InFlight:     values[8],
		IoTicks:      values[9],
		TimeInQueue:  values[10],
	}, nil
}

// getInstalledBus resolves the sysfs symlink for blockName and extracts the
// PCIe bus address, SATA port, USB port, or NVMe slot from the path.
func getInstalledBus(blockName string) (*InstallPosition, error) {
	blockName = strings.TrimPrefix(blockName, "/dev/")
	linkPath := fmt.Sprintf("/sys/block/%s", blockName)
	realPath, err := os.Readlink(linkPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read symlink for block device: %w", err)
	}

	parts := strings.Split(realPath, "/")
	var pcieBusAddress, sataPort, usbPort, nvmeSlot string
	for i, part := range parts {
		switch {
		case strings.HasPrefix(part, "pci"):
			pcieBusAddress = part
		case strings.HasPrefix(part, "ata"):
			sataPort = part
		case strings.HasPrefix(part, "usb"):
			if i+1 < len(parts) && strings.Contains(parts[i+1], ":") {
				usbPort = parts[i]
			}
		case strings.HasPrefix(part, "nvme"):
			if i+1 < len(parts) && strings.HasPrefix(parts[i+1], "nvme") {
				nvmeSlot = parts[i+1]
			}
		}
	}

	if pcieBusAddress == "" && sataPort == "" && usbPort == "" && nvmeSlot == "" {
		return nil, fmt.Errorf("failed to extract bus info from sysfs path: %s", realPath)
	}

	return &InstallPosition{
		PCIEBusAddress: pcieBusAddress,
		SATAPort:       sataPort,
		USBPort:        usbPort,
		NVMESlot:       nvmeSlot,
	}, nil
}
