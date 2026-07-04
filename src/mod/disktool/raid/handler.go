package raid

import (
	"encoding/json"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"imuslab.com/bokodm/bokodmd/mod/disktool/diskfs"
	"imuslab.com/bokodm/bokodmd/mod/utils"
)

/*
	Handler.go

	This module handle api call to the raid module
*/

// Handle remove a member disk (sdX) from RAID volume (mdX)
func (m *Manager) HandleRemoveDiskFromRAIDVol(w http.ResponseWriter, r *http.Request) {
	//mdadm --remove /dev/md0 /dev/sdb1
	mdDev, err := utils.PostPara(r, "raidDev")
	if err != nil {
		utils.SendErrorResponse(w, "invalid raid device given")
		return
	}

	sdXDev, err := utils.PostPara(r, "memDev")
	if err != nil {
		utils.SendErrorResponse(w, "invalid member device given")
		return
	}

	//Check if target array exists
	if !m.RAIDDeviceExists(mdDev) {
		utils.SendErrorResponse(w, "target RAID array not exists")
		return
	}

	//Check if this is the only disk in the array
	if !m.IsSafeToRemove(mdDev, sdXDev) {
		utils.SendErrorResponse(w, "removal of this device will cause data loss")
		return
	}

	//Check if the disk is already failed
	diskAlreadyFailed, err := m.DiskIsFailed(mdDev, sdXDev)
	if err != nil {
		log.Println("[RAID] Unable to validate if disk failed: " + err.Error())
		utils.SendErrorResponse(w, err.Error())
		return
	}
	//Disk not failed. Mark it as failed
	if !diskAlreadyFailed {
		err = m.FailDisk(mdDev, sdXDev)
		if err != nil {
			utils.SendErrorResponse(w, err.Error())
			return
		}
	}

	//Add some delay for OS level to handle IO closing
	time.Sleep(300 * time.Millisecond)

	//Done. Remove the device from array
	err = m.RemoveDisk(mdDev, sdXDev)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}
	log.Println("[RAID] Memeber disk " + sdXDev + " removed from RAID volume " + mdDev)
	utils.SendOK(w)
}

// Handle marking a member disk (sdX) as failed in RAID volume (mdX)
// This is the first step of a disk swap operation: the disk is marked
// as faulty so mdadm stops writing to it, but it stays in the array
// until it is removed with the remove API.
func (m *Manager) HandleFailDisk(w http.ResponseWriter, r *http.Request) {
	//mdadm /dev/md0 --fail /dev/sdb1
	mdDev, err := utils.PostPara(r, "raidDev")
	if err != nil {
		utils.SendErrorResponse(w, "invalid raid device given")
		return
	}

	sdXDev, err := utils.PostPara(r, "memDev")
	if err != nil {
		utils.SendErrorResponse(w, "invalid member device given")
		return
	}

	//Check if target array exists
	if !m.RAIDDeviceExists(mdDev) {
		utils.SendErrorResponse(w, "target RAID array not exists")
		return
	}

	//Removal safety check also applies to failing a disk: failing the
	//last data-holding disk of an array kills the array
	if !m.IsSafeToRemove(mdDev, sdXDev) {
		utils.SendErrorResponse(w, "marking this device as failed will cause data loss")
		return
	}

	diskAlreadyFailed, err := m.DiskIsFailed(mdDev, sdXDev)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}
	if diskAlreadyFailed {
		utils.SendErrorResponse(w, "target device is already marked as failed")
		return
	}

	err = m.FailDisk(mdDev, sdXDev)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	log.Println("[RAID] Member disk " + sdXDev + " marked as failed in RAID volume " + mdDev)
	utils.SendOK(w)
}

// Handle listing disks that can be added to a RAID volume as a new
// member or hot spare. A disk qualifies when it is a physical disk,
// carries no mounted filesystem (itself or any of its partitions) and is
// not a member of any RAID array.
func (m *Manager) HandleListAddCandidates(w http.ResponseWriter, r *http.Request) {
	storageDevices, err := diskfs.ListAllStorageDevices()
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	//Collect the name of every RAID member device (can be sdX or sdX1)
	raidMembers := map[string]bool{}
	raidPools, err := m.GetRAIDDevicesFromProcMDStat()
	if err == nil {
		for _, md := range raidPools {
			for _, member := range md.Members {
				raidMembers[member.Name] = true
			}
		}
	}

	candidates := []diskfs.BlockDeviceMeta{}
	for _, device := range storageDevices.Blockdevices {
		if device.Type != "disk" {
			continue
		}
		//Skip virtual / non-storage devices
		if strings.HasPrefix(device.Name, "loop") || strings.HasPrefix(device.Name, "zram") || strings.HasPrefix(device.Name, "sr") || strings.HasPrefix(device.Name, "md") {
			continue
		}
		if raidMembers[device.Name] {
			continue
		}

		inUse := device.Mountpoint != ""
		for _, child := range device.Children {
			if child.Mountpoint != "" || raidMembers[child.Name] {
				inUse = true
				break
			}
		}
		if inUse {
			continue
		}

		candidates = append(candidates, device)
	}

	js, _ := json.Marshal(candidates)
	utils.SendJSONResponse(w, string(js))
}

// Handle adding a disk (mdX) to RAID volume (mdX)
func (m *Manager) HandleAddDiskToRAIDVol(w http.ResponseWriter, r *http.Request) {
	//mdadm --add /dev/md0 /dev/sdb1
	mdDev, err := utils.PostPara(r, "raidDev")
	if err != nil {
		utils.SendErrorResponse(w, "invalid raid device given")
		return
	}

	sdXDev, err := utils.PostPara(r, "memDev")
	if err != nil {
		utils.SendErrorResponse(w, "invalid member device given")
		return
	}

	//Check if target array exists
	if !m.RAIDDeviceExists(mdDev) {
		utils.SendErrorResponse(w, "target RAID array not exists")
		return
	}

	//Check if disk already in another RAID array or mounted
	isMounted, err := diskfs.DeviceIsMounted(sdXDev)
	if err != nil {
		utils.SendErrorResponse(w, "unable to read device state")
		return
	}

	if isMounted {
		utils.SendErrorResponse(w, "target device is mounted")
		return
	}

	diskUsedByAnotherRAID, err := m.DiskIsUsedInAnotherRAIDVol(sdXDev)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	if diskUsedByAnotherRAID {
		utils.SendErrorResponse(w, "target device already been used by another RAID volume")
		return
	}

	isOSDisk, err := m.DiskIsRoot(sdXDev)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	if isOSDisk {
		utils.SendErrorResponse(w, "OS disk cannot be used as RAID member")
		return
	}

	//Reject disks that contain any mounted partition: they are either in
	//use or serving system paths like /boot
	bdMeta, err := diskfs.GetBlockDeviceMeta(sdXDev)
	if err == nil {
		for _, child := range bdMeta.Children {
			if child.Mountpoint != "" {
				utils.SendErrorResponse(w, "target disk contains mounted partitions and cannot be used as RAID member")
				return
			}
		}
	}

	//OK! Clear the disk
	err = m.ClearSuperblock(sdXDev)
	if err != nil {
		utils.SendErrorResponse(w, "unable to clear superblock of device")
		return
	}

	//Add it to the target RAID array
	err = m.AddDisk(mdDev, sdXDev)
	if err != nil {
		utils.SendErrorResponse(w, "adding disk to RAID volume failed")
		return
	}

	log.Println("[RAID] Device " + sdXDev + " added to RAID volume " + mdDev)

	utils.SendOK(w)
}

// Handle force flush reloading mdadm to solve the md0 become md127 problem
func (m *Manager) HandleMdadmFlushReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		//This force the request to pass through the csrf check
		utils.SendErrorResponse(w, "invalid method")
		return
	}
	err := m.FlushReload()
	if err != nil {
		utils.SendErrorResponse(w, "reload failed: "+strings.ReplaceAll(err.Error(), "\n", " "))
		return
	}
	utils.SendOK(w)
}

// Handle resolving the disk model label, might return null
func (m *Manager) HandleResolveDiskModelLabel(w http.ResponseWriter, r *http.Request) {
	devName, err := utils.GetPara(r, "devName")
	if err != nil {
		utils.SendErrorResponse(w, "invalid device name given")
		return
	}

	//Function only accept sdX not /dev/sdX
	devName = filepath.Base(devName)

	labelSize, labelModel, err := diskfs.GetDiskModelByName(devName)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	js, _ := json.Marshal([]string{labelModel, labelSize})
	utils.SendJSONResponse(w, string(js))
}

// Handle force flush reloading mdadm to solve the md0 become md127 problem
func (m *Manager) HandlListChildrenDeviceInfo(w http.ResponseWriter, r *http.Request) {
	devName, err := utils.GetPara(r, "devName")
	if err != nil {
		utils.SendErrorResponse(w, "invalid device name given")
		return
	}

	if !strings.HasPrefix(devName, "/dev/") {
		devName = "/dev/" + devName
	}

	//Get the children devices for this RAID
	raidDevice, err := m.GetRAIDDeviceByDevicePath(devName)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	//Merge the child devices info into one array
	results := map[string]*diskfs.BlockDeviceMeta{}
	for _, blockdevice := range raidDevice.Members {
		bdm, err := diskfs.GetBlockDeviceMeta("/dev/" + blockdevice.Name)
		if err != nil {
			log.Println("[RAID] Unable to load block device info: " + err.Error())
			results[blockdevice.Name] = &diskfs.BlockDeviceMeta{
				Name: blockdevice.Name,
				Size: -1,
			}

			continue
		}

		results[blockdevice.Name] = bdm
	}

	js, _ := json.Marshal(results)
	utils.SendJSONResponse(w, string(js))
}

// Handle list all the disks that is usable
func (m *Manager) HandleListUsableDevices(w http.ResponseWriter, r *http.Request) {
	storageDevices, err := diskfs.ListAllStorageDevices()
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	//Filter out the block devices that are disks
	usableDisks := []diskfs.BlockDeviceMeta{}
	for _, device := range storageDevices.Blockdevices {
		if device.Type == "disk" {
			usableDisks = append(usableDisks, device)
		}
	}

	js, _ := json.Marshal(usableDisks)
	utils.SendJSONResponse(w, string(js))

}

// Handle loading the detail of a given RAID array
func (m *Manager) HandleLoadArrayDetail(w http.ResponseWriter, r *http.Request) {
	devName, err := utils.GetPara(r, "dev")
	if err != nil {
		utils.SendErrorResponse(w, "invalid device name given")
		return
	}

	if !strings.HasPrefix(devName, "/dev/") {
		devName = "/dev/" + devName
	}

	//Check device exists
	if !utils.FileExists(devName) {
		utils.SendErrorResponse(w, "target device not exists")
		return
	}

	//Get status of the array
	targetRAIDInfo, err := m.GetRAIDInfo(devName)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	js, _ := json.Marshal(targetRAIDInfo)
	utils.SendJSONResponse(w, string(js))
}

// Handle formating a device
func (m *Manager) HandleFormatRaidDevice(w http.ResponseWriter, r *http.Request) {
	devName, err := utils.GetPara(r, "devName")
	if err != nil {
		utils.SendErrorResponse(w, "invalid device name given")
		return
	}

	format, err := utils.GetPara(r, "format")
	if err != nil {
		utils.SendErrorResponse(w, "invalid device name given")
		return
	}

	if !strings.HasPrefix(devName, "/dev/") {
		devName = "/dev/" + devName
	}

	//Check if the target device exists
	if !m.RAIDDeviceExists(devName) {
		utils.SendErrorResponse(w, "target not exists or not a valid RAID device")
		return
	}

	//Format the drive
	err = diskfs.FormatStorageDevice(format, devName)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	utils.SendOK(w)
}

// List all the raid device in this system
func (m *Manager) HandleListRaidDevices(w http.ResponseWriter, r *http.Request) {
	rdevs, err := m.GetRAIDDevicesFromProcMDStat()
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	results := []*RAIDInfo{}
	for _, rdev := range rdevs {
		arrayInfo, err := m.GetRAIDInfo("/dev/" + rdev.Name)
		if err != nil {
			continue
		}

		results = append(results, arrayInfo)
	}

	js, _ := json.Marshal(results)
	utils.SendJSONResponse(w, string(js))
}

// Create a RAID storage pool
func (m *Manager) HandleCreateRAIDDevice(w http.ResponseWriter, r *http.Request) {
	devName, err := utils.PostPara(r, "devName")
	if err != nil || devName == "" {
		//Use auto generated one
		devName, err = GetNextAvailableMDDevice()
		if err != nil {
			utils.SendErrorResponse(w, err.Error())
			return
		}
	}
	raidName, err := utils.PostPara(r, "raidName")
	if err != nil {
		utils.SendErrorResponse(w, "invalid raid storage name given")
		return
	}
	raidLevelStr, err := utils.PostPara(r, "level")
	if err != nil {
		utils.SendErrorResponse(w, "invalid raid level given")
		return
	}

	raidDevicesJSON, err := utils.PostPara(r, "raidDev")
	if err != nil {
		utils.SendErrorResponse(w, "invalid raid device array given")
		return
	}

	spareDevicesJSON, err := utils.PostPara(r, "spareDev")
	if err != nil {
		utils.SendErrorResponse(w, "invalid spare device array given")
		return
	}

	//Get if superblock require all zeroed (will also do formating after raid constructed)
	zerosuperblock, err := utils.PostBool(r, "zerosuperblock")
	if err != nil {
		zerosuperblock = false
	}

	//Convert raidDevices and spareDevices ID into string slice
	raidDevices := []string{}
	spareDevices := []string{}

	err = json.Unmarshal([]byte(raidDevicesJSON), &raidDevices)
	if err != nil {
		utils.SendErrorResponse(w, "unable to parse raid device into array")
		return
	}

	err = json.Unmarshal([]byte(spareDevicesJSON), &spareDevices)
	if err != nil {
		utils.SendErrorResponse(w, "unable to parse spare devices into array")
		return
	}

	//Make sure RAID Name do not contain spaces or werid charcters
	if strings.Contains(raidName, " ") {
		utils.SendErrorResponse(w, "raid name cannot contain space")
		return
	}

	//Convert raidLevel to int
	raidLevelStr = strings.TrimPrefix(raidLevelStr, "raid")
	raidLevel, err := strconv.Atoi(raidLevelStr)
	if err != nil {
		utils.SendErrorResponse(w, "invalid raid level given")
		return
	}

	if zerosuperblock {
		//Format each drives
		drivesToZeroblocks := []string{}
		for _, raidDev := range raidDevices {
			if !strings.HasPrefix(raidDev, "/dev/") {
				//Prepend /dev/ to it if not set
				raidDev = filepath.Join("/dev/", raidDev)
			}

			if !utils.FileExists(raidDev) {
				//This disk not found
				utils.SendErrorResponse(w, raidDev+" not found")
				return
			}

			thisDisk := raidDev
			drivesToZeroblocks = append(drivesToZeroblocks, thisDisk)
		}

		for _, spareDev := range spareDevices {
			if !strings.HasPrefix(spareDev, "/dev/") {
				//Prepend /dev/ to it if not set
				spareDev = filepath.Join("/dev/", spareDev)
			}

			if !utils.FileExists(spareDev) {
				//This disk not found
				utils.SendErrorResponse(w, spareDev+" not found")
				return
			}

			thisDisk := spareDev
			drivesToZeroblocks = append(drivesToZeroblocks, thisDisk)
		}

		for _, clearPendingDisk := range drivesToZeroblocks {
			//Format all drives
			log.Println("RAID", "Clearning superblock for disk "+clearPendingDisk, nil)
			err = m.ClearSuperblock(clearPendingDisk)
			if err != nil {
				log.Println("RAID", "Unable to format "+clearPendingDisk+": "+err.Error(), err)
				utils.SendErrorResponse(w, err.Error())
				return
			}
		}
	}

	//Create the RAID device
	err = m.CreateRAIDDevice(devName, raidName, raidLevel, raidDevices, spareDevices)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	//Update the mdadm config
	err = m.UpdateMDADMConfig()
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	utils.SendOK(w)
}

// Request to reload the RAID manager and scan new / fix missing raid pools
func (m *Manager) HandleRaidDevicesAssemble(w http.ResponseWriter, r *http.Request) {
	err := m.RestartRAIDService()
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	utils.SendOK(w)
}

// Remove a given raid device with its name, USE WITH CAUTION
func (m *Manager) HandleRemoveRaideDevice(w http.ResponseWriter, r *http.Request) {
	targetDevice, err := utils.PostPara(r, "raidDev")
	if err != nil {
		utils.SendErrorResponse(w, "target device not given")
		return
	}

	//Check if the raid device exists
	if !m.RAIDDeviceExists(targetDevice) {
		utils.SendErrorResponse(w, "target device not exists")
		return
	}

	//Get the RAID device memeber disks
	targetRAIDDevice, err := m.GetRAIDDeviceByDevicePath(targetDevice)
	if err != nil {
		utils.SendErrorResponse(w, "error occured when trying to load target RAID device info")
		return
	}

	//Check if it is mounted. If yes, unmount it
	if !strings.HasPrefix(targetDevice, "/dev/") {
		targetDevice = filepath.Join("/dev/", targetDevice)
	}

	mounted, err := diskfs.DeviceIsMounted(targetDevice)
	if err != nil {
		log.Println("RAID", "Unmount failed: "+err.Error(), err)
		utils.SendErrorResponse(w, err.Error())
		return
	}

	if mounted {
		log.Println("RAID", targetDevice+" is mounted. Trying to unmount...", nil)
		err = diskfs.UnmountDevice(targetDevice)
		if err != nil {
			log.Println("[RAID] Unmount failed: " + err.Error())
			utils.SendErrorResponse(w, err.Error())
			return
		}

		//Wait for 3 seconds to check if it is still mounted
		counter := 0
		for counter < 3 {
			mounted, _ := diskfs.DeviceIsMounted(targetDevice)
			if mounted {
				//Still not unmounted. Wait for it
				log.Println("RAID", "Device still mounted. Retrying in 1 second", nil)
				counter++
				time.Sleep(1 * time.Second)
			} else {
				break
			}
		}

		//Check if it is still mounted
		mounted, _ = diskfs.DeviceIsMounted(targetDevice)
		if mounted {
			utils.SendErrorResponse(w, "unmount RAID partition failed: device is busy")
			return
		}
	}

	//Give it some time for the raid device to finish umount
	time.Sleep(300 * time.Millisecond)

	//Stop & Remove RAID service on the target device
	err = m.StopRAIDDevice(targetDevice)
	if err != nil {
		log.Println("RAID", "Stop RAID partition failed: "+err.Error(), err)
		utils.SendErrorResponse(w, err.Error())
		return
	}

	//Zeroblock the RAID device member disks
	for _, memberDisk := range targetRAIDDevice.Members {
		//Member disk name do not contain full path
		name := memberDisk.Name
		if !strings.HasPrefix(name, "/dev/") {
			name = filepath.Join("/dev/", name)
		}

		err = m.ClearSuperblock(name)
		if err != nil {
			log.Println("RAID", "Unable to clear superblock on device "+name, err)
			continue
		}
	}

	//Update the mdadm config
	err = m.UpdateMDADMConfig()
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	//Done
	utils.SendOK(w)
}

// Force reload all RAID config from file
func (m *Manager) HandleForceAssembleReload(w http.ResponseWriter, r *http.Request) {
	err := m.FlushReload()
	if err != nil {
		log.Println("RAID", "mdadm reload failed: "+err.Error(), err)
		utils.SendErrorResponse(w, err.Error())
		return
	}

	utils.SendOK(w)
}

// Grow the raid array to maxmium possible size of the current disks
func (m *Manager) HandleGrowRAIDArray(w http.ResponseWriter, r *http.Request) {
	deviceName, err := utils.PostPara(r, "raidDev")
	if err != nil {
		utils.SendErrorResponse(w, "raid device not given")
		return
	}

	//mdadm --detail requires the full device path
	if !strings.HasPrefix(deviceName, "/dev/") {
		deviceName = filepath.Join("/dev/", deviceName)
	}

	if !m.RAIDDeviceExists(deviceName) {
		utils.SendErrorResponse(w, "target raid device not exists")
		return
	}

	//Check the raid is healthy and ok for expansion
	raidNotHealthy, err := m.RAIDArrayContainsFailedDisks(deviceName)
	if err != nil {
		utils.SendErrorResponse(w, "unable to check health state before expansion")
		return
	}
	if raidNotHealthy {
		utils.SendErrorResponse(w, "expand can only be performed on a healthy array")
		return
	}

	//Expand the raid array
	err = m.GrowRAIDDevice(deviceName)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	err = m.RestartRAIDService()
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	utils.SendOK(w)
}

// HandleRenderOverview List the info and health of all loaded RAID array
func (m *Manager) HandleRenderOverview(w http.ResponseWriter, r *http.Request) {
	//Get all raid device from procmd
	rdevs, err := m.GetRAIDDevicesFromProcMDStat()
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	type RaidHealthOverview struct {
		Name      string
		Status    string
		Level     string
		UsedSize  int64
		TotalSize int64
		IsHealthy bool
	}

	results := []*RaidHealthOverview{}

	//Get RAID Status for each devices
	for _, raidDev := range rdevs {
		//Fill in the basic information
		thisRaidOverview := RaidHealthOverview{
			Name:      raidDev.Name,
			Status:    raidDev.Status,
			Level:     raidDev.Level,
			UsedSize:  -1,
			TotalSize: -1,
			IsHealthy: false,
		}

		//Get health status of RAID
		raidPath := filepath.Join("/dev/", strings.TrimPrefix(raidDev.Name, "/dev/"))
		raidStatus, err := GetRAIDStatus(raidPath)
		if err == nil {
			thisRaidOverview.IsHealthy = raidStatus.isHealthy()
		}

		// Get RAID vol size and info
		raidPartitionSize, err := GetRAIDPartitionSize(raidPath)
		if err == nil {
			thisRaidOverview.TotalSize = raidPartitionSize
		}

		raidUsedSize, err := GetRAIDUsedSize(raidPath)
		if err == nil {
			thisRaidOverview.UsedSize = raidUsedSize
		}

		results = append(results, &thisRaidOverview)
	}

	js, _ := json.Marshal(results)
	utils.SendJSONResponse(w, string(js))
}

/* Sync State Related Features */
// HandleGetRAIDSyncState Get the sync state of a given RAID device
func (m *Manager) HandleGetRAIDSyncState(w http.ResponseWriter, r *http.Request) {
	devName, err := utils.GetPara(r, "dev")
	if err != nil {
		utils.SendErrorResponse(w, "invalid device name given")
		return
	}

	if !strings.HasPrefix(devName, "/dev/") {
		devName = filepath.Join("/dev/", devName)
	}

	//Get the sync state of the RAID device
	syncState, err := m.GetSyncStateByPath(devName)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	js, _ := json.Marshal(syncState)
	utils.SendJSONResponse(w, string(js))
}

// HandleSyncPendingToReadWrite Set the pending sync to read-write mode
// to reactivate the resync process
func (m *Manager) HandleSyncPendingToReadWrite(w http.ResponseWriter, r *http.Request) {
	devName, err := utils.PostPara(r, "dev")
	if err != nil {
		utils.SendErrorResponse(w, "invalid device name given")
		return
	}

	if !strings.HasPrefix(devName, "/dev/") {
		devName = filepath.Join("/dev/", devName)
	}

	//Set the pending sync to read-write mode
	err = m.SetSyncPendingToReadWrite(devName)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	utils.SendOK(w)
}
