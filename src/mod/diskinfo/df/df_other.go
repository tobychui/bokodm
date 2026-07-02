//go:build !linux && !darwin
// +build !linux,!darwin

package df

import "errors"

func getDiskUsageByPath(_ string) (*DiskInfo, error) {
	return nil, errors.New("disk usage queries are not supported on this platform")
}

func getDiskUsage() ([]DiskInfo, error) {
	return nil, errors.New("disk usage queries are not supported on this platform")
}
