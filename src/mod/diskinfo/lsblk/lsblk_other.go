//go:build !linux && !darwin
// +build !linux,!darwin

package lsblk

import "errors"

func getLSBLKOutput() ([]BlockDevice, error) {
	return nil, errors.New("block device enumeration is not supported on this platform")
}

func getBlockDeviceInfoFromDevicePath(_ string) (*BlockDevice, error) {
	return nil, errors.New("block device enumeration is not supported on this platform")
}
