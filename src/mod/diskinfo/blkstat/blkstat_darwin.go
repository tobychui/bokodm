//go:build darwin
// +build darwin

package blkstat

import (
	"fmt"
	"os/exec"
	"strings"

	gopsutil_disk "github.com/shirou/gopsutil/v4/disk"
)

func getBlockStat(blockName string) (*BlockStat, error) {
	blockName = strings.TrimPrefix(blockName, "/dev/")

	counters, err := gopsutil_disk.IOCounters(blockName)
	if err != nil {
		return nil, fmt.Errorf("failed to get I/O counters: %w", err)
	}

	counter, ok := counters[blockName]
	if !ok {
		return nil, fmt.Errorf("disk %s not found in I/O counters", blockName)
	}

	return &BlockStat{
		ReadIOs:      counter.ReadCount,
		ReadMerges:   0,
		ReadSectors:  counter.ReadBytes / 512,
		ReadTicks:    counter.ReadTime,
		WriteIOs:     counter.WriteCount,
		WriteMerges:  0,
		WriteSectors: counter.WriteBytes / 512,
		WriteTicks:   counter.WriteTime,
		InFlight:     0,
		IoTicks:      counter.IoTime,
		TimeInQueue:  0,
	}, nil
}

func getInstalledBus(blockName string) (*InstallPosition, error) {
	blockName = strings.TrimPrefix(blockName, "/dev/")

	cmd := exec.Command("bash", "-c",
		fmt.Sprintf(`ioreg -r -c IOMedia -l | grep -A 30 '"BSD Name" = "%s"'`, blockName))
	output, err := cmd.Output()
	if err != nil || len(output) == 0 {
		return nil, fmt.Errorf("could not find ioreg entry for %s", blockName)
	}

	pos := &InstallPosition{}
	outStr := string(output)

	switch {
	case strings.Contains(outStr, "Thunderbolt") || strings.Contains(outStr, "AppleThunderbolt"):
		pos.PCIEBusAddress = "Thunderbolt"
	case strings.Contains(outStr, "USB"):
		pos.USBPort = "USB"
	case strings.Contains(outStr, "NVMe") || strings.Contains(outStr, "AppleNVMeController"):
		pos.NVMESlot = "NVMe"
	case strings.Contains(outStr, "SATA") || strings.Contains(outStr, "AHCI"):
		pos.SATAPort = "SATA"
	default:
		pos.PCIEBusAddress = "PCIe"
	}

	return pos, nil
}
