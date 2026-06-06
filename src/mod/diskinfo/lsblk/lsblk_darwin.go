//go:build darwin
// +build darwin

package lsblk

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// ---- Apple plist XML parser ----

func plistParseDict(decoder *xml.Decoder) (map[string]interface{}, error) {
	result := make(map[string]interface{})
	for {
		t, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		switch tok := t.(type) {
		case xml.StartElement:
			if tok.Name.Local == "key" {
				var key string
				if err := decoder.DecodeElement(&key, &tok); err != nil {
					return nil, err
				}
				value, err := plistNextValue(decoder)
				if err != nil {
					return nil, err
				}
				result[key] = value
			}
		case xml.EndElement:
			if tok.Name.Local == "dict" {
				return result, nil
			}
		}
	}
}

func plistParseArray(decoder *xml.Decoder) ([]interface{}, error) {
	var result []interface{}
	for {
		t, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		switch tok := t.(type) {
		case xml.StartElement:
			v, err := plistParseElement(decoder, tok)
			if err != nil {
				return nil, err
			}
			result = append(result, v)
		case xml.EndElement:
			if tok.Name.Local == "array" {
				return result, nil
			}
		}
	}
}

func plistNextValue(decoder *xml.Decoder) (interface{}, error) {
	for {
		t, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		if se, ok := t.(xml.StartElement); ok {
			return plistParseElement(decoder, se)
		}
	}
}

func plistParseElement(decoder *xml.Decoder, se xml.StartElement) (interface{}, error) {
	switch se.Name.Local {
	case "dict":
		return plistParseDict(decoder)
	case "array":
		return plistParseArray(decoder)
	case "string":
		var s string
		if err := decoder.DecodeElement(&s, &se); err != nil {
			return nil, err
		}
		return s, nil
	case "integer":
		var s string
		if err := decoder.DecodeElement(&s, &se); err != nil {
			return nil, err
		}
		n, _ := strconv.ParseInt(s, 10, 64)
		return n, nil
	case "true":
		_ = decoder.Skip()
		return true, nil
	case "false":
		_ = decoder.Skip()
		return false, nil
	default:
		_ = decoder.Skip()
		return nil, nil
	}
}

// parseDiskutilPlist converts `diskutil list -plist` XML output to BlockDevice structs.
func parseDiskutilPlist(data []byte) ([]BlockDevice, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))

	var rootDict map[string]interface{}
	for {
		t, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("failed reading plist: %w", err)
		}
		if se, ok := t.(xml.StartElement); ok && se.Name.Local == "dict" {
			rootDict, err = plistParseDict(decoder)
			if err != nil {
				return nil, fmt.Errorf("failed parsing plist dict: %w", err)
			}
			break
		}
	}

	allRaw, ok := rootDict["AllDisksAndPartitions"]
	if !ok {
		return nil, fmt.Errorf("AllDisksAndPartitions not found in diskutil plist")
	}
	allDisksRaw, ok := allRaw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected type for AllDisksAndPartitions")
	}

	wholeDiskNames := map[string]bool{}
	if wdRaw, ok := rootDict["WholeDisks"]; ok {
		if wdArr, ok := wdRaw.([]interface{}); ok {
			for _, v := range wdArr {
				if s, ok := v.(string); ok {
					wholeDiskNames[s] = true
				}
			}
		}
	}

	var devices []BlockDevice
	for _, raw := range allDisksRaw {
		diskDict, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		devID, _ := diskDict["DeviceIdentifier"].(string)
		size, _ := diskDict["Size"].(int64)

		deviceType := "part"
		if wholeDiskNames[devID] {
			deviceType = "disk"
		}

		dev := BlockDevice{Name: devID, Size: size, Type: deviceType}

		if partRaw, ok := diskDict["Partitions"]; ok {
			if partArr, ok := partRaw.([]interface{}); ok {
				for _, pRaw := range partArr {
					pDict, ok := pRaw.(map[string]interface{})
					if !ok {
						continue
					}
					pID, _ := pDict["DeviceIdentifier"].(string)
					pSize, _ := pDict["Size"].(int64)
					mountPoint, _ := pDict["MountPoint"].(string)
					dev.Children = append(dev.Children, BlockDevice{
						Name:       pID,
						Size:       pSize,
						Type:       "part",
						MountPoint: mountPoint,
					})
				}
			}
		}
		devices = append(devices, dev)
	}
	return devices, nil
}

func getLSBLKOutput() ([]BlockDevice, error) {
	cmd := exec.Command("diskutil", "list", "-plist")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("diskutil list failed: %w", err)
	}
	return parseDiskutilPlist(out.Bytes())
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
