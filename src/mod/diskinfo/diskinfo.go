package diskinfo

import (
	"errors"
	"os"
	"strings"

	"imuslab.com/bokodm/bokodmd/mod/diskinfo/blkid"
	"imuslab.com/bokodm/bokodmd/mod/diskinfo/lsblk"
)

// Get a disk by its device path, accept both /dev/sda and sda
func NewBlockFromDevicePath(devpath string) (*Block, error) {
	if !strings.HasPrefix(devpath, "/dev/") {
		devpath = "/dev/" + devpath
	}

	if _, err := os.Stat(devpath); errors.Is(err, os.ErrNotExist) {
		return nil, errors.New("device path does not exist")
	}

	//Create a new disk object
	thisDisk := &Block{
		Path: devpath,
	}

	//Try to get the block device info
	err := thisDisk.UpdateProperties()
	if err != nil {
		return nil, err
	}

	return thisDisk, nil
}

// UpdateProperties updates the properties of the disk.
func (d *Block) UpdateProperties() error {
	//Try to get the block device info
	blockDeviceInfo, err := lsblk.GetBlockDeviceInfoFromDevicePath(d.Path)
	if err != nil {
		return err
	}

	// Update the disk properties
	d.Name = blockDeviceInfo.Name
	d.Size = blockDeviceInfo.Size
	d.BlockType = blockDeviceInfo.Type
	//d.MountPoint = blockDeviceInfo.MountPoint

	if d.BlockType == "disk" {
		//This block is a disk not a partition. There is no partition ID info
		//So we can skip the blkid call
		return nil
	}

	// Get the partition ID
	diskIdInfo, err := blkid.GetPartitionIDFromDevicePath(d.Path)
	if err != nil {
		return err
	}

	// Update the disk properties with ID info
	d.UUID = diskIdInfo.UUID
	d.FsType = diskIdInfo.Type
	d.BlockSize = diskIdInfo.BlockSize
	return nil
}
