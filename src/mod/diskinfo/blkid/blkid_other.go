//go:build !linux && !darwin
// +build !linux,!darwin

package blkid

import "errors"

func getPartitionIdInfo() ([]BlockDevice, error) {
	return nil, errors.New("partition identification is not supported on this platform")
}

func getPartitionIDFromDevicePath(_ string) (*BlockDevice, error) {
	return nil, errors.New("partition identification is not supported on this platform")
}
