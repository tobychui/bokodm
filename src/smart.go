package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"imuslab.com/bokodm/bokodmd/mod/diskinfo"
	"imuslab.com/bokodm/bokodmd/mod/diskinfo/smart"
	"imuslab.com/bokodm/bokodmd/mod/utils"
)

/*
	smart.go

	This file handles the SMART management and monitoring API routing

	Support APIs

	/smart/health/{diskname} - Get the health status of a disk
	/smart/health/all - Get the health status of all disks
	/smart/info/{diskname} - Get the SMART information of a disk
*/

// Handler for SMART API calls
func HandleSMARTCalls() http.Handler {
	return http.StripPrefix("/smart/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pathParts := strings.Split(r.URL.Path, "/")
		if len(pathParts) < 2 {
			http.Error(w, "Bad Request - Missing disk name", http.StatusBadRequest)
			return
		}
		subPath := pathParts[0]
		diskName := pathParts[1]
		if diskName == "" {
			http.Error(w, "Bad Request - Missing disk name", http.StatusBadRequest)
			return
		}
		switch subPath {
		case "health":
			if diskName == "all" {
				// Get the SMART information for all disks
				allDisks, err := diskinfo.GetAllDisks()
				if err != nil {
					log.Println("Error getting all disks:", err)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
					return
				}

				// Create a map to hold the SMART information for each disk
				diskInfoMap := []*smart.DriveHealthInfo{}
				for _, disk := range allDisks {
					diskName := disk.Name
					health, err := smart.GetDiskSMARTHealthSummary(diskName)
					if err != nil {
						log.Println("Error getting disk health:", err)
						continue
					}

					diskInfoMap = append(diskInfoMap, health)
				}
				// Convert the disk information to JSON and write it to the response
				js, _ := json.Marshal(diskInfoMap)
				utils.SendJSONResponse(w, string(js))
				return
			}

			// Get the health status of the disk
			health, err := smart.GetDiskSMARTHealthSummary(diskName)
			if err != nil {
				log.Println("Error getting disk health:", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			// Convert the health status to JSON and write it to the response
			js, _ := json.Marshal(health)
			utils.SendJSONResponse(w, string(js))
			return
		case "info":
			// Handle SMART API calls
			dt, err := smart.GetDiskType(diskName)
			if err != nil {
				log.Println("Error getting disk type:", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}

			if dt == smart.DiskType_SATA {
				// Get SATA disk information
				sataInfo, err := smart.GetSATAInfo(diskName)
				if err != nil {
					log.Println("Error getting SATA disk info:", err)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
					return
				}

				// Convert the SATA info to JSON and write it to the response
				js, _ := json.Marshal(sataInfo)
				utils.SendJSONResponse(w, string(js))
			} else if dt == smart.DiskType_NVMe {
				// Get NVMe disk information
				nvmeInfo, err := smart.GetNVMEInfo(diskName)
				if err != nil {
					log.Println("Error getting NVMe disk info:", err)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
					return
				}

				// Convert the NVMe info to JSON and write it to the response
				js, _ := json.Marshal(nvmeInfo)
				utils.SendJSONResponse(w, string(js))
			} else {
				log.Println("Unknown disk type:", dt)
				http.Error(w, "Bad Request - Unknown disk type", http.StatusBadRequest)
				return
			}
			return
		default:
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
	}))
}
