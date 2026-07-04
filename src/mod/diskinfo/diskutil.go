package diskinfo

import (
	"errors"
	"path/filepath"
	"strings"

	"log"

	"imuslab.com/bokodm/bokodmd/mod/diskinfo/blkid"
	"imuslab.com/bokodm/bokodmd/mod/diskinfo/df"
	"imuslab.com/bokodm/bokodmd/mod/diskinfo/fdisk"
	"imuslab.com/bokodm/bokodmd/mod/diskinfo/lsblk"
)

// GetAllDisks retrieves all disks on the system.
func GetAllDisks() ([]*Disk, error) {
	allBlockDevices, err := lsblk.GetLSBLKOutput()
	if err != nil {
		return nil, err
	}

	disks := []*Disk{}

	for _, blockDevice := range allBlockDevices {
		if blockDevice.Type == "disk" {
			thisDisk, err := GetDiskInfo(blockDevice.Name)
			if err != nil {
				return nil, err
			}
			disks = append(disks, thisDisk)
		}
	}

	return disks, nil
}

// DevicePathIsValidDisk checks if the given device path is a disk.
func DevicePathIsValidDisk(path string) bool {
	//Make sure the path has a prefix and a trailing slash
	if !strings.HasPrefix(path, "/dev/") {
		path = "/dev/" + path
	}

	path = strings.TrimSuffix(path, "/")

	allBlockDevices, err := lsblk.GetLSBLKOutput()
	if err != nil {
		log.Println("Error getting block devices:", err)
		return false
	}

	for _, blockDevice := range allBlockDevices {
		if "/dev/"+blockDevice.Name == path {
			return blockDevice.Type == "disk"
		}
	}

	return false
}

// DevicePathIsPartition checks if the given device path is a valid partition.
// The block device tree is searched recursively because devices can nest
// more than one level deep (e.g. sdb → md0 → md0p1).
func DevicePathIsValidPartition(path string) bool {
	//Make sure the path has a prefix and a trailing slash
	if !strings.HasPrefix(path, "/dev/") {
		path = "/dev/" + path
	}

	path = strings.TrimSuffix(path, "/")

	allBlockDevices, err := lsblk.GetLSBLKOutput()
	if err != nil {
		return false
	}

	//Any device that appears below the top level qualifies: partitions
	//(sda1), RAID volumes (md0) and partitions on RAID volumes (md0p1)
	var findInChildren func(devices []lsblk.BlockDevice) bool
	findInChildren = func(devices []lsblk.BlockDevice) bool {
		for _, device := range devices {
			if "/dev/"+device.Name == path {
				return true
			}
			if findInChildren(device.Children) {
				return true
			}
		}
		return false
	}

	for _, blockDevice := range allBlockDevices {
		if findInChildren(blockDevice.Children) {
			return true
		}
	}

	return false
}

// GetDiskInfo retrieves the disk information for a given disk name.
// e.g. "sda"
// for partitions, use the GetPartitionInfo function
func GetDiskInfo(diskname string) (*Disk, error) {
	if diskname == "" {
		return nil, errors.New("disk name is empty")
	}
	//Make sure the diskname is something like sda
	diskname = strings.TrimPrefix(diskname, "/dev/")

	//Create a new disk object
	thisDisk := &Disk{
		Name:       diskname,
		Size:       0,
		BlockType:  "disk",
		Partitions: []*Partition{},
	}

	//Try to get the disk model and identifier
	diskInfo, err := fdisk.GetDiskModelAndIdentifier(diskname)
	if err == nil {
		thisDisk.Model = diskInfo.Model
		thisDisk.Identifier = diskInfo.Identifier
		thisDisk.DiskLabel = diskInfo.DiskLabel
	}

	//Calculation variables for total disk used space
	totalDiskUseSpace := int64(0)

	//Populate the partitions
	allBlockDevices, err := lsblk.GetLSBLKOutput()
	if err != nil {
		return nil, err
	}

	//Find the device anywhere in the tree: md RAID volumes are nested
	//under their member disks instead of being top-level entries
	var findDevice func(devices []lsblk.BlockDevice) *lsblk.BlockDevice
	findDevice = func(devices []lsblk.BlockDevice) *lsblk.BlockDevice {
		for i := range devices {
			if devices[i].Name == diskname {
				return &devices[i]
			}
			if match := findDevice(devices[i].Children); match != nil {
				return match
			}
		}
		return nil
	}

	if blockDevice := findDevice(allBlockDevices); blockDevice != nil {
		thisDisk.Size = blockDevice.Size
		for _, partition := range blockDevice.Children {
			//Get the partition information from blkid
			partition := &Partition{
				Name:       partition.Name,
				Size:       partition.Size,
				Path:       filepath.Join("/dev", partition.Name),
				BlockType:  partition.Type,
				MountPoint: partition.MountPoint,
			}

			//Get the partition ID
			blkInfo, err := blkid.GetPartitionIDFromDevicePath(partition.Name)
			if err == nil {
				partition.UUID = blkInfo.UUID
				partition.PartUUID = blkInfo.PartUUID
				partition.PartLabel = blkInfo.PartLabel
				partition.BlockSize = blkInfo.BlockSize
				partition.BlockType = blkInfo.Type
				partition.FsType = blkInfo.Type
			}

			//Get the disk usage information
			diskUsage, err := df.GetDiskUsageByPath(partition.Name)
			if err == nil {
				partition.Used = diskUsage.Used
				partition.Free = diskUsage.Available
			}

			thisDisk.Partitions = append(thisDisk.Partitions, partition)
		}
	}

	//Calculate the total disk used space
	for _, partition := range thisDisk.Partitions {
		totalDiskUseSpace += partition.Used
	}
	thisDisk.Used = totalDiskUseSpace
	thisDisk.Free = thisDisk.Size - totalDiskUseSpace
	return thisDisk, nil
}

func GetPartitionInfo(partitionName string) (*Partition, error) {
	partition := &Partition{
		Name: partitionName,
	}
	partInfo, err := blkid.GetPartitionIDFromDevicePath(partitionName)
	if err == nil {
		partition.UUID = partInfo.UUID
		partition.PartUUID = partInfo.PartUUID
		partition.PartLabel = partInfo.PartLabel
		partition.BlockSize = partInfo.BlockSize
		partition.BlockType = partInfo.Type
		partition.FsType = partInfo.Type
	}
	//Get the disk usage information
	diskUsage, err := df.GetDiskUsageByPath(partitionName)
	if err == nil {
		partition.Used = diskUsage.Used
		partition.Free = diskUsage.Available
		partition.MountPoint = diskUsage.MountedOn

	}

	return partition, nil
}

// GetDevicePathFromPartitionID retrieves the device path for a given partition ID.
func GetDevicePathFromPartitionID(diskID string) (string, error) {
	if diskID == "" {
		return "", errors.New("disk ID is empty")
	}

	// Try to get the block device info
	allBlockDevices, err := lsblk.GetLSBLKOutput()
	if err != nil {
		return "", err
	}

	for _, blockDevice := range allBlockDevices {
		//Check each of the children to see if there is a partition with the given ID
		for _, child := range blockDevice.Children {
			if child.Name == diskID {
				return child.Name, nil
			}
		}
	}

	return "", errors.New("disk ID not found")
}
