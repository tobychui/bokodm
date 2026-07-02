//go:build !linux && !darwin
// +build !linux,!darwin

package fdisk

import "errors"

func getDiskModelAndIdentifier(_ string) (*DiskInfo, error) {
	return nil, errors.New("disk model/label lookup is not supported on this platform")
}
