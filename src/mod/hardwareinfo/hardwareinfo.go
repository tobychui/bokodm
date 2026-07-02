package hardwareinfo

/*
	hardwareinfo.go

	Public API and shared types for hardware information queries.
	sysinfo_linux.go   — Linux implementation (ip, lscpu, /proc)
	sysinfo_darwin.go  — macOS implementation (sysctl, system_profiler)
	sysinfo_other.go   — stubs for unsupported platforms
*/

import (
	"encoding/json"
	"log"
	"net/http"

	"imuslab.com/bokofs/bokofsd/mod/utils"
)

// ---- Shared types ----

type CPUInfo struct {
	Model       string
	Freq        string
	Instruction string
	Hardware    string
	Revision    string
}

type LogicalDisk struct {
	DriveLetter string
	FileSystem  string
	FreeSpace   string
}

type ArOZInfo struct {
	BuildVersion string
	DeviceVendor string
	DeviceModel  string
	VendorIcon   string
	SN           string
	HostOS       string
	CPUArch      string
	HostName     string
}

type Server struct {
	hostInfo ArOZInfo
}

func NewInfoServer(a ArOZInfo) *Server {
	return &Server{hostInfo: a}
}

func (s *Server) GetArOZInfo(w http.ResponseWriter, r *http.Request) {
	jsonData, err := json.Marshal(s.hostInfo)
	if err != nil {
		log.Println(err)
		return
	}

	loadImage, _ := utils.GetPara(r, "icon")
	if loadImage != "true" {
		t := ArOZInfo{}
		json.Unmarshal(jsonData, &t)
		t.VendorIcon = ""
		jsonData, _ = json.Marshal(t)
	}
	utils.SendJSONResponse(w, string(jsonData))
}

// ---- Public HTTP handler wrappers ----

// Ifconfig returns a JSON list of network-interface output lines.
func Ifconfig(w http.ResponseWriter, r *http.Request) {
	sysIfconfig(w, r)
}

// GetDriveStat returns a JSON list of mounted filesystem usage.
func GetDriveStat(w http.ResponseWriter, r *http.Request) {
	sysGetDriveStat(w, r)
}

// GetUSB returns a JSON list of connected USB devices.
func GetUSB(w http.ResponseWriter, r *http.Request) {
	sysGetUSB(w, r)
}

// GetCPUInfo returns a JSON CPUInfo object.
func GetCPUInfo(w http.ResponseWriter, r *http.Request) {
	sysGetCPUInfo(w, r)
}

// GetRamInfo returns installed RAM in bytes as a JSON integer.
func GetRamInfo(w http.ResponseWriter, r *http.Request) {
	sysGetRamInfo(w, r)
}
