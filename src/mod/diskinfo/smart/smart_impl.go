package smart

/*
	smart_impl.go

	Shared smartctl-based implementations used on every supported platform.
	Disk-type detection is platform-specific and lives in smart_linux.go /
	smart_other.go.
*/

import (
	"bufio"
	"errors"
	"os/exec"
	"strconv"
	"strings"
)

func getNVMEInfo(disk string) (*NVMEInfo, error) {
	if !strings.HasPrefix(disk, "/dev/") {
		disk = "/dev/" + disk
	}

	output, err := exec.Command("smartctl", "-i", disk).Output()
	if err != nil {
		return nil, err
	}

	info := &NVMEInfo{}
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "Model Number:"):
			info.ModelNumber = strings.TrimSpace(strings.TrimPrefix(line, "Model Number:"))
		case strings.HasPrefix(line, "Serial Number:"):
			info.SerialNumber = strings.TrimSpace(strings.TrimPrefix(line, "Serial Number:"))
		case strings.HasPrefix(line, "Firmware Version:"):
			info.FirmwareVersion = strings.TrimSpace(strings.TrimPrefix(line, "Firmware Version:"))
		case strings.HasPrefix(line, "PCI Vendor/Subsystem ID:"):
			info.PCIVendorSubsystemID = strings.TrimSpace(strings.TrimPrefix(line, "PCI Vendor/Subsystem ID:"))
		case strings.HasPrefix(line, "IEEE OUI Identifier:"):
			info.IEEEOUIIdentifier = strings.TrimSpace(strings.TrimPrefix(line, "IEEE OUI Identifier:"))
		case strings.HasPrefix(line, "Total NVM Capacity:"):
			info.TotalNVMeCapacity = strings.TrimSpace(strings.TrimPrefix(line, "Total NVM Capacity:"))
		case strings.HasPrefix(line, "Unallocated NVM Capacity:"):
			info.UnallocatedNVMeCapacity = strings.TrimSpace(strings.TrimPrefix(line, "Unallocated NVM Capacity:"))
		case strings.HasPrefix(line, "Controller ID:"):
			info.ControllerID = strings.TrimSpace(strings.TrimPrefix(line, "Controller ID:"))
		case strings.HasPrefix(line, "NVMe Version:"):
			info.NVMeVersion = strings.TrimSpace(strings.TrimPrefix(line, "NVMe Version:"))
		case strings.HasPrefix(line, "Number of Namespaces:"):
			info.NumberOfNamespaces = strings.TrimSpace(strings.TrimPrefix(line, "Number of Namespaces:"))
		case strings.HasPrefix(line, "Namespace 1 Size/Capacity:"):
			info.NamespaceSizeCapacity = strings.TrimSpace(strings.TrimPrefix(line, "Namespace 1 Size/Capacity:"))
		case strings.HasPrefix(line, "Namespace 1 Utilization:"):
			info.NamespaceUtilization = strings.TrimSpace(strings.TrimPrefix(line, "Namespace 1 Utilization:"))
		case strings.HasPrefix(line, "Namespace 1 Formatted LBA Size:"):
			info.NamespaceFormattedLBASize = strings.TrimSpace(strings.TrimPrefix(line, "Namespace 1 Formatted LBA Size:"))
		case strings.HasPrefix(line, "Namespace 1 IEEE EUI-64:"):
			info.NamespaceIEEE_EUI_64 = strings.TrimSpace(strings.TrimPrefix(line, "Namespace 1 IEEE EUI-64:"))
		}
	}
	return info, scanner.Err()
}

func getSATAInfo(disk string) (*SATADiskInfo, error) {
	if !strings.HasPrefix(disk, "/dev/") {
		disk = "/dev/" + disk
	}

	output, err := exec.Command("smartctl", "-i", disk).Output()
	if err != nil {
		return nil, err
	}

	info := &SATADiskInfo{}
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "Model Family:"):
			info.ModelFamily = strings.TrimSpace(strings.TrimPrefix(line, "Model Family:"))
		case strings.HasPrefix(line, "Device Model:"):
			info.DeviceModel = strings.TrimSpace(strings.TrimPrefix(line, "Device Model:"))
		case strings.HasPrefix(line, "Serial Number:"):
			info.SerialNumber = strings.TrimSpace(strings.TrimPrefix(line, "Serial Number:"))
		case strings.HasPrefix(line, "Firmware Version:"):
			info.Firmware = strings.TrimSpace(strings.TrimPrefix(line, "Firmware Version:"))
		case strings.HasPrefix(line, "User Capacity:"):
			info.UserCapacity = strings.TrimSpace(strings.TrimPrefix(line, "User Capacity:"))
		case strings.HasPrefix(line, "Sector Size:"):
			info.SectorSize = strings.TrimSpace(strings.TrimPrefix(line, "Sector Size:"))
		case strings.HasPrefix(line, "Rotation Rate:"):
			info.RotationRate = strings.TrimSpace(strings.TrimPrefix(line, "Rotation Rate:"))
		case strings.HasPrefix(line, "Form Factor:"):
			info.FormFactor = strings.TrimSpace(strings.TrimPrefix(line, "Form Factor:"))
		case strings.HasPrefix(line, "SMART support is:"):
			info.SmartSupport = strings.TrimSpace(strings.TrimPrefix(line, "SMART support is:")) == "Enabled"
		}
	}
	return info, scanner.Err()
}

func setSmartEnable(disk string, isEnabled bool) error {
	if !strings.HasPrefix(disk, "/dev/") {
		disk = "/dev/" + disk
	}
	enableCmd := "off"
	if isEnabled {
		enableCmd = "on"
	}

	output, err := exec.Command("smartctl", "-s", enableCmd, disk).Output()
	if err != nil {
		return err
	}
	if strings.Contains(string(output), "SMART Enabled") {
		return nil
	}
	println(string(output))
	return errors.New("failed to enable SMART on disk")
}

func getDiskSmartCheck(diskname string) (*SMARTTestResult, error) {
	if !strings.HasPrefix(diskname, "/dev/") {
		diskname = "/dev/" + diskname
	}

	output, err := exec.Command("smartctl", "-H", "-A", diskname).Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 32 {
			// Exit code 32 is non-critical for some drives; continue with output
		} else {
			println(string(output))
			return nil, err
		}
	}

	result := &SMARTTestResult{
		TestResult:         "Unknown",
		MarginalAttributes: make([]SMARTAttribute, 0),
	}
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	inAttributesSection := false

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "SMART overall-health self-assessment test result:") {
			result.TestResult = strings.TrimSpace(strings.TrimPrefix(line, "SMART overall-health self-assessment test result:"))
		}
		if strings.HasPrefix(line, "ID# ATTRIBUTE_NAME") {
			inAttributesSection = true
			continue
		}
		if inAttributesSection {
			fields := strings.Fields(line)
			if len(fields) >= 10 {
				id, err := strconv.Atoi(fields[0])
				if err != nil {
					continue
				}
				value, _ := strconv.Atoi(fields[3])
				worst, _ := strconv.Atoi(fields[4])
				threshold, _ := strconv.Atoi(fields[5])

				result.MarginalAttributes = append(result.MarginalAttributes, SMARTAttribute{
					ID:         id,
					Name:       fields[1],
					Flag:       fields[2],
					Value:      value,
					Worst:      worst,
					Threshold:  threshold,
					Type:       fields[6],
					Updated:    fields[7],
					WhenFailed: fields[8],
					RawValue:   strings.Join(fields[9:], " "),
				})
			}
		}
	}

	if err := scanner.Err(); err != nil {
		println(string(output))
		return nil, err
	}
	if result.TestResult == "" {
		return nil, errors.New("unable to determine SMART health status")
	}
	return result, nil
}

func getDiskSmartHealthSummary(diskname string) (*DriveHealthInfo, error) {
	smartCheck, err := getDiskSmartCheck(diskname)
	if err != nil {
		return nil, err
	}

	healthInfo := &DriveHealthInfo{
		DeviceName: diskname,
		IsHealthy:  strings.ToUpper(smartCheck.TestResult) == "PASSED",
	}

	dt, err := getDiskType(diskname)
	if err != nil {
		return nil, err
	}

	switch dt {
	case DiskType_SATA:
		sataInfo, err := getSATAInfo(diskname)
		if err != nil {
			return nil, err
		}
		healthInfo.DeviceModel = sataInfo.DeviceModel
		healthInfo.SerialNumber = sataInfo.SerialNumber
		healthInfo.IsSSD = strings.Contains(sataInfo.RotationRate, "Solid State")
	case DiskType_NVMe:
		nvmeInfo, err := getNVMEInfo(diskname)
		if err != nil {
			return nil, err
		}
		healthInfo.DeviceModel = nvmeInfo.ModelNumber
		healthInfo.SerialNumber = nvmeInfo.SerialNumber
		healthInfo.IsNVMe = true
	default:
		return nil, errors.New("unsupported disk type")
	}

	for _, attr := range smartCheck.MarginalAttributes {
		switch attr.Name {
		case "Power_On_Hours":
			if v, err := strconv.ParseUint(attr.RawValue, 10, 64); err == nil {
				healthInfo.PowerOnHours = v
			}
		case "Power_Cycle_Count":
			if v, err := strconv.ParseUint(attr.RawValue, 10, 64); err == nil {
				healthInfo.PowerCycleCount = v
			}
		case "Reallocated_Sector_Ct":
			if v, err := strconv.ParseUint(attr.RawValue, 10, 64); err == nil {
				healthInfo.ReallocatedSectors = v
			}
		case "Wear_Leveling_Count":
			if v, err := strconv.ParseUint(attr.RawValue, 10, 64); err == nil {
				healthInfo.WearLevelingCount = v
			}
		case "Uncorrectable_Error_Cnt":
			if v, err := strconv.ParseUint(attr.RawValue, 10, 64); err == nil {
				healthInfo.UncorrectableErrors = v
			}
		case "Current_Pending_Sector":
			if v, err := strconv.ParseUint(attr.RawValue, 10, 64); err == nil {
				healthInfo.PendingSectors = v
			}
		case "ECC_Recovered":
			if v, err := strconv.ParseUint(attr.RawValue, 10, 64); err == nil {
				healthInfo.ECCRecovered = v
			}
		case "UDMA_CRC_Error_Count":
			if v, err := strconv.ParseUint(attr.RawValue, 10, 64); err == nil {
				healthInfo.UDMACRCErrors = v
			}
		case "Total_LBAs_Written":
			if v, err := strconv.ParseUint(attr.RawValue, 10, 64); err == nil {
				healthInfo.TotalLBAWritten = v
			}
		case "Total_LBAs_Read":
			if v, err := strconv.ParseUint(attr.RawValue, 10, 64); err == nil {
				healthInfo.TotalLBARead = v
			}
		}
	}
	return healthInfo, nil
}
