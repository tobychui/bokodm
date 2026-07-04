package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"imuslab.com/bokodm/bokodmd/mod/diskinfo"
	"imuslab.com/bokodm/bokodmd/mod/diskinfo/lsblk"
	"imuslab.com/bokodm/bokodmd/mod/netstat"
)

/*
	API Router

	This module handle routing of the API calls
*/

// Primary handler for the API router
func HandlerAPIcalls() http.Handler {
	return http.StripPrefix("/api", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get the disk ID from the URL path
		pathParts := strings.Split(r.URL.Path, "/")
		if len(pathParts) < 2 {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}

		diskID := pathParts[1]
		if diskID == "" {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}

		switch diskID {
		case "info":
			// Request to /api/info/*
			HandleInfoAPIcalls().ServeHTTP(w, r)
			return
		case "smart":
			// Request to /api/smart/*
			HandleSMARTCalls().ServeHTTP(w, r)
			return
		case "raid":
			// Request to /api/raid/*
			HandleRAIDCalls().ServeHTTP(w, r)
			return
		case "netmount":
			// Request to /api/netmount/*
			HandleNetMountCalls().ServeHTTP(w, r)
			return
		case "disks":
			// Request to /api/disks/* (partition mount tools)
			HandleDiskMountCalls().ServeHTTP(w, r)
			return
		case "parttool":
			// Request to /api/parttool/* (partitioning & formatting)
			HandlePartToolCalls().ServeHTTP(w, r)
			return
		case "settings":
			// Request to /api/settings/*
			HandleSettingsCalls().ServeHTTP(w, r)
			return
		case "logs":
			// Request to /api/logs/* (log store for the Logs tab)
			HandleLogsCalls().ServeHTTP(w, r)
		default:
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
	}))
}

// Handler for info API calls
func HandleInfoAPIcalls() http.Handler {
	return http.StripPrefix("/info/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		//Check the next part of the URL
		pathParts := strings.Split(r.URL.Path, "/")
		subPath := pathParts[0]
		switch subPath {
		case "deps":
			// Return the runtime dependency report as JSON.
			// The frontend uses this to gate features that need missing tools.
			js, _ := json.Marshal(runtimeDeps)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(js)
			return
		case "report":
			// Generate the full system analytic report
			HandleAnalyticReport(w, r)
			return
		case "netstat":
			// Get the current network statistics
			netstatBuffer.HandleGetBufferedNetworkInterfaceStats(w, r)
			return
		case "diskio":
			// Get the live per-disk IO throughput
			HandleDiskIORates(w, r)
			return
		case "iface":
			// Get the list of network interfaces
			netstat.HandleListNetworkInterfaces(w, r)
			return
		case "list":
			// List all block devices and their partitions
			blockDevices, err := lsblk.GetLSBLKOutput()
			if err != nil {
				log.Println("Error getting block devices:", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}

			disks := make([]*diskinfo.Disk, 0)
			for _, device := range blockDevices {
				if device.Type == "disk" {
					disk, err := diskinfo.GetDiskInfo(device.Name)
					if err != nil {
						log.Println("Error getting disk info:", err)
						http.Error(w, "Internal Server Error", http.StatusInternalServerError)
						return
					}
					disks = append(disks, disk)
				}
			}

			// md RAID volumes are nested under their member disks in the
			// lsblk tree; surface each assembled array as its own entry so
			// its partitions can be inspected and mounted like a real disk
			seenMdDevices := map[string]bool{}
			var collectMdDevices func(devices []lsblk.BlockDevice)
			collectMdDevices = func(devices []lsblk.BlockDevice) {
				for _, device := range devices {
					if strings.HasPrefix(device.Type, "raid") && strings.HasPrefix(device.Name, "md") && !seenMdDevices[device.Name] {
						seenMdDevices[device.Name] = true
						disk, err := diskinfo.GetDiskInfo(device.Name)
						if err == nil {
							if disk.Model == "" {
								disk.Model = "RAID Volume (" + device.Type + ")"
							}
							disks = append(disks, disk)
						}
					}
					collectMdDevices(device.Children)
				}
			}
			for _, device := range blockDevices {
				collectMdDevices(device.Children)
			}
			// Convert the block devices to JSON and write it to the response
			js, _ := json.Marshal(disks)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(js)
		case "disk":
			// Get the disk info for a particular disk, e.g. sda
			if len(pathParts) < 2 {
				http.Error(w, "Bad Request - Invalid disk name", http.StatusBadRequest)
				return
			}
			diskID := pathParts[1]
			if diskID == "" {
				http.Error(w, "Bad Request - Invalid disk name", http.StatusBadRequest)
				return
			}

			if !diskinfo.DevicePathIsValidDisk(diskID) {
				log.Println("Invalid disk ID:", diskID)
				http.Error(w, "Bad Request", http.StatusBadRequest)
				return
			}

			// Handle diskinfo API calls
			targetDiskInfo, err := diskinfo.GetDiskInfo(diskID)
			if err != nil {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}

			// Convert the disk info to JSON and write it to the response
			js, _ := json.Marshal(targetDiskInfo)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(js)
			return
		case "part":
			// Get the partition info for a particular partition, e.g. sda1
			if len(pathParts) < 2 {
				http.Error(w, "Bad Request - Missing parition name", http.StatusBadRequest)
				return
			}
			partID := pathParts[1]
			if partID == "" {
				http.Error(w, "Bad Request - Missing parition name", http.StatusBadRequest)
				return
			}

			if !diskinfo.DevicePathIsValidPartition(partID) {
				log.Println("Invalid partition name:", partID)
				http.Error(w, "Bad Request - Invalid parition name", http.StatusBadRequest)
				return
			}

			// Handle partinfo API calls
			targetPartInfo, err := diskinfo.GetPartitionInfo(partID)
			if err != nil {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}

			// Convert the partition info to JSON and write it to the response
			js, _ := json.Marshal(targetPartInfo)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(js)

			return
		default:
			fmt.Println("Unknown API call:", subPath)
			http.Error(w, "Not Found", http.StatusNotFound)
		}

	}))
}
