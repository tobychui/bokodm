//go:build darwin
// +build darwin

package blkid

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"imuslab.com/bokofs/bokofsd/mod/diskinfo/lsblk"
)

// ---- minimal Apple plist XML parser ----

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

// diskutilInfoPlist runs `diskutil info -plist <devPath>` and returns the parsed dict.
func diskutilInfoPlist(devPath string) (map[string]interface{}, error) {
	cmd := exec.Command("diskutil", "info", "-plist", devPath)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("diskutil info failed for %s: %w", devPath, err)
	}

	decoder := xml.NewDecoder(bytes.NewReader(out.Bytes()))
	for {
		t, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("failed reading diskutil info plist: %w", err)
		}
		if se, ok := t.(xml.StartElement); ok && se.Name.Local == "dict" {
			return plistParseDict(decoder)
		}
	}
}

func blockDeviceFromInfoDict(devPath string, info map[string]interface{}) *BlockDevice {
	bd := &BlockDevice{Device: devPath}
	if v, ok := info["VolumeUUID"].(string); ok {
		bd.UUID = v
	}
	if v, ok := info["FilesystemType"].(string); ok {
		bd.Type = v
	} else if v, ok := info["FilesystemName"].(string); ok {
		bd.Type = strings.ToLower(v)
	} else if v, ok := info["Content"].(string); ok {
		bd.Type = strings.ToLower(v)
	}
	if v, ok := info["DeviceBlockSize"].(int64); ok {
		bd.BlockSize = int(v)
	}
	if bd.UUID == "" {
		if v, ok := info["DiskUUID"].(string); ok {
			bd.UUID = v
		}
	}
	if v, ok := info["PartitionUUID"].(string); ok {
		bd.PartUUID = v
	}
	if v, ok := info["VolumeName"].(string); ok {
		bd.PartLabel = v
	}
	return bd
}

func getPartitionIdInfo() ([]BlockDevice, error) {
	allDevices, err := lsblk.GetLSBLKOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to enumerate devices: %w", err)
	}

	var result []BlockDevice
	for _, disk := range allDevices {
		for _, part := range disk.Children {
			info, err := diskutilInfoPlist("/dev/" + part.Name)
			if err != nil {
				continue
			}
			result = append(result, *blockDeviceFromInfoDict("/dev/"+part.Name, info))
		}
	}
	return result, nil
}

func getPartitionIDFromDevicePath(devpath string) (*BlockDevice, error) {
	devpath = strings.TrimPrefix(devpath, "/dev/")
	if strings.Contains(devpath, "/") {
		return nil, errors.New("invalid device path")
	}

	info, err := diskutilInfoPlist("/dev/" + devpath)
	if err != nil {
		return nil, fmt.Errorf("failed to get partition info for /dev/%s: %w", devpath, err)
	}
	return blockDeviceFromInfoDict("/dev/"+devpath, info), nil
}
