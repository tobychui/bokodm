package main

import (
	"encoding/json"
	"log"
	"net/http"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
	"imuslab.com/bokodm/bokodmd/mod/diskinfo"
	"imuslab.com/bokodm/bokodmd/mod/diskinfo/smart"
	"imuslab.com/bokodm/bokodmd/mod/disktool/raid"
	"imuslab.com/bokodm/bokodmd/mod/netstat"
	"imuslab.com/bokodm/bokodmd/mod/utils"
)

/*
	report.go

	This file generates the full system analytic report served at
	GET /api/info/report. The report aggregates everything this system
	knows about the host: OS info, hardware info, network interfaces,
	disks, partitions, SMART data and RAID arrays. The frontend
	(web/report.html) renders it as a printable HTML table page that
	can be exported to PDF via the browser print dialog.
*/

// HostReport contains the OS and hardware information of this host.
type HostReport struct {
	Hostname        string `json:"hostname"`
	OS              string `json:"os"`
	Platform        string `json:"platform"`
	PlatformVersion string `json:"platformVersion"`
	KernelVersion   string `json:"kernelVersion"`
	KernelArch      string `json:"kernelArch"`
	UptimeSec       uint64 `json:"uptimeSec"`
	BootTime        uint64 `json:"bootTime"`
	CPUModel        string `json:"cpuModel"`
	CPUCores        int    `json:"cpuCores"`
	TotalMemory     uint64 `json:"totalMemory"`
	GoVersion       string `json:"goVersion"`
}

// DiskReport bundles everything known about a single disk.
type DiskReport struct {
	Disk        *diskinfo.Disk         `json:"disk"`
	SmartHealth *smart.DriveHealthInfo `json:"smartHealth,omitempty"`
	SmartInfo   interface{}            `json:"smartInfo,omitempty"` // *smart.SATADiskInfo or *smart.NVMEInfo
	SmartType   string                 `json:"smartType,omitempty"` // "sata" or "nvme"
}

// AnalyticReport is the full report structure returned by /api/info/report.
type AnalyticReport struct {
	GeneratedAt       time.Time                  `json:"generatedAt"`
	SystemUUID        string                     `json:"systemUUID"`
	Host              *HostReport                `json:"host"`
	NetworkInterfaces []netstat.NetworkInterface `json:"networkInterfaces"`
	Disks             []*DiskReport              `json:"disks"`
	RAIDArrays        []*raid.RAIDInfo           `json:"raidArrays"`
	Dependencies      *DependencyReport          `json:"dependencies"`
}

func buildHostReport() *HostReport {
	report := &HostReport{
		OS:        runtime.GOOS,
		GoVersion: runtime.Version(),
	}

	if hostInfo, err := host.Info(); err == nil {
		report.Hostname = hostInfo.Hostname
		report.Platform = hostInfo.Platform
		report.PlatformVersion = hostInfo.PlatformVersion
		report.KernelVersion = hostInfo.KernelVersion
		report.KernelArch = hostInfo.KernelArch
		report.UptimeSec = hostInfo.Uptime
		report.BootTime = hostInfo.BootTime
	}

	if cpuInfos, err := cpu.Info(); err == nil && len(cpuInfos) > 0 {
		report.CPUModel = cpuInfos[0].ModelName
	}
	if cores, err := cpu.Counts(true); err == nil {
		report.CPUCores = cores
	}

	if vm, err := mem.VirtualMemory(); err == nil {
		report.TotalMemory = vm.Total
	}

	return report
}

func buildDiskReports() []*DiskReport {
	reports := []*DiskReport{}

	allDisks, err := diskinfo.GetAllDisks()
	if err != nil {
		log.Println("[Report] Unable to list disks:", err)
		return reports
	}

	smartAvailable := runtimeDeps.IsFeatureAvailable("smart")
	for _, disk := range allDisks {
		thisReport := &DiskReport{
			Disk: disk,
		}

		if smartAvailable {
			if health, err := smart.GetDiskSMARTHealthSummary(disk.Name); err == nil {
				thisReport.SmartHealth = health
			}

			if dt, err := smart.GetDiskType(disk.Name); err == nil {
				if dt == smart.DiskType_SATA {
					if info, err := smart.GetSATAInfo(disk.Name); err == nil {
						thisReport.SmartInfo = info
						thisReport.SmartType = "sata"
					}
				} else if dt == smart.DiskType_NVMe {
					if info, err := smart.GetNVMEInfo(disk.Name); err == nil {
						thisReport.SmartInfo = info
						thisReport.SmartType = "nvme"
					}
				}
			}
		}

		reports = append(reports, thisReport)
	}

	return reports
}

func buildRAIDReports() []*raid.RAIDInfo {
	arrays := []*raid.RAIDInfo{}
	if raidManager == nil {
		return arrays
	}

	rdevs, err := raidManager.GetRAIDDevicesFromProcMDStat()
	if err != nil {
		log.Println("[Report] Unable to list RAID devices:", err)
		return arrays
	}

	for _, rdev := range rdevs {
		arrayInfo, err := raidManager.GetRAIDInfo("/dev/" + rdev.Name)
		if err != nil {
			continue
		}
		arrays = append(arrays, arrayInfo)
	}

	return arrays
}

// HandleAnalyticReport generates and returns the full system analytic report
func HandleAnalyticReport(w http.ResponseWriter, r *http.Request) {
	ifaces, err := netstat.ListNetworkInterfaces()
	if err != nil {
		ifaces = []netstat.NetworkInterface{}
	}

	report := AnalyticReport{
		GeneratedAt:       time.Now(),
		SystemUUID:        sysuuid,
		Host:              buildHostReport(),
		NetworkInterfaces: ifaces,
		Disks:             buildDiskReports(),
		RAIDArrays:        buildRAIDReports(),
		Dependencies:      runtimeDeps,
	}

	// Server-side PDF export, use ?format=pdf
	format, _ := utils.GetPara(r, "format")
	if format == "pdf" {
		servePDFReport(w, &report)
		return
	}

	js, err := json.Marshal(report)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	utils.SendJSONResponse(w, string(js))
}
