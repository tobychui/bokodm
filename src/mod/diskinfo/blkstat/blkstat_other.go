//go:build !linux && !darwin
// +build !linux,!darwin

package blkstat

import "errors"

func getBlockStat(_ string) (*BlockStat, error) {
	return nil, errors.New("blkstat is not supported on this platform")
}

func getInstalledBus(_ string) (*InstallPosition, error) {
	return nil, errors.New("blkstat is not supported on this platform")
}
