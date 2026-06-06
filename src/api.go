package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"imuslab.com/bokofs/bokofsd/mod/diskinfo"
	"imuslab.com/bokofs/bokofsd/mod/diskinfo/lsblk"
	"imuslab.com/bokofs/bokofsd/mod/netstat"
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
		case "netstat":
			// Get the current network statistics
			netstatBuffer.HandleGetBufferedNetworkInterfaceStats(w, r)
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
