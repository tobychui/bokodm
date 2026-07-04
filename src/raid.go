package main

import (
	"net/http"
	"strings"
)

/*
	raid.go

	This file handles the RAID management and monitoring API routing
*/

func HandleRAIDCalls() http.Handler {
	return http.StripPrefix("/raid/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if raidManager == nil {
			http.Error(w, "RAID management is not available on this platform", http.StatusServiceUnavailable)
			return
		}
		pathParts := strings.Split(r.URL.Path, "/")

		switch pathParts[0] {
		case "list":
			// List all RAID devices
			raidManager.HandleListRaidDevices(w, r)
			return
		case "info":
			// Handle loading the detail of a given RAID array, require "dev=md0" as a query parameter
			raidManager.HandleLoadArrayDetail(w, r)
			return
		case "create":
			// Create a new RAID device, devName (Optional, e.g. md0), raidName (e.g. myraid), level (e.g. 0),
			// raidDev(e.g. sda,sdb), spareDev(e.g. sdc), zerosuperblock(bool)
			raidManager.HandleCreateRAIDDevice(w, r)
			return
		case "overview":
			// Render the RAID overview page
			raidManager.HandleRenderOverview(w, r)
			return
		case "sync":
			// Get the RAID sync state, require "dev=md0" as a query parameter
			raidManager.HandleGetRAIDSyncState(w, r)
			return
		case "start-resync":
			// Activate a RAID device, require "dev=md0" as a query parameter
			raidManager.HandleSyncPendingToReadWrite(w, r)
			return
		case "reassemble":
			// Reassemble all RAID devices
			raidManager.HandleForceAssembleReload(w, r)
			return
		case "delete":
			// Delete a RAID device, require "raidDev=md0" as a query parameter
			raidManager.HandleRemoveRaideDevice(w, r)
			return
		case "add":
			// Add a new disk to the RAID device, require POST "raidDev=md0" and "memDev=sdX"
			raidManager.HandleAddDiskToRAIDVol(w, r)
			return
		case "remove":
			// Remove a member disk from the RAID device (fails it first when needed),
			// require POST "raidDev=md0" and "memDev=sdX"
			raidManager.HandleRemoveDiskFromRAIDVol(w, r)
			return
		case "fail":
			// Mark a member disk as failed (step 1 of a disk swap),
			// require POST "raidDev=md0" and "memDev=sdX"
			raidManager.HandleFailDisk(w, r)
			return
		case "grow":
			// Grow the RAID array to the maximum size of the current disks,
			// require POST "raidDev=md0"
			raidManager.HandleGrowRAIDArray(w, r)
			return
		case "format":
			// Format the RAID device, require "devName=md0" and "format=ext4"
			raidManager.HandleFormatRaidDevice(w, r)
			return
		case "candidates":
			// List disks that can be added to a RAID volume as member / spare
			raidManager.HandleListAddCandidates(w, r)
			return
		case "children":
			// List block device info of all member disks, require "devName=md0"
			raidManager.HandlListChildrenDeviceInfo(w, r)
			return
		case "label":
			// Resolve the disk model label, require "devName=sdX"
			raidManager.HandleResolveDiskModelLabel(w, r)
			return
		default:
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
	}))
}
