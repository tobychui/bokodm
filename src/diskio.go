package main

import (
	"encoding/json"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/disk"
	"imuslab.com/bokodm/bokodmd/mod/utils"
)

/*
	diskio.go

	Background sampler for per-disk IO throughput. A goroutine polls the
	kernel IO counters once per second and keeps the latest byte-per-second
	rates, served at GET /api/info/diskio for the status page monitor.
*/

// DiskIORate is the live throughput of one block device.
type DiskIORate struct {
	Name      string `json:"name"`
	ReadBps   int64  `json:"readBps"`
	WriteBps  int64  `json:"writeBps"`
	ReadOps   int64  `json:"readOps"`  // IOPS
	WriteOps  int64  `json:"writeOps"` // IOPS
	SizeTotal int64  `json:"-"`
}

var (
	diskIORates   = map[string]*DiskIORate{}
	diskIORatesMu sync.RWMutex

	// Whole physical disks and md volumes only, no partitions
	diskIONamePattern = regexp.MustCompile(`^(sd[a-z]+|hd[a-z]+|vd[a-z]+|xvd[a-z]+|md[0-9]+|nvme[0-9]+n[0-9]+|mmcblk[0-9]+)$`)
)

// startDiskIOSampler launches the background sampling loop.
func startDiskIOSampler() {
	go func() {
		var lastCounters map[string]disk.IOCountersStat
		lastSample := time.Now()

		for {
			counters, err := disk.IOCounters()
			now := time.Now()
			if err == nil && lastCounters != nil {
				elapsed := now.Sub(lastSample).Seconds()
				if elapsed > 0 {
					newRates := map[string]*DiskIORate{}
					for name, current := range counters {
						if !diskIONamePattern.MatchString(name) {
							continue
						}
						previous, ok := lastCounters[name]
						if !ok {
							continue
						}
						newRates[name] = &DiskIORate{
							Name:     name,
							ReadBps:  int64(float64(current.ReadBytes-previous.ReadBytes) / elapsed),
							WriteBps: int64(float64(current.WriteBytes-previous.WriteBytes) / elapsed),
							ReadOps:  int64(float64(current.ReadCount-previous.ReadCount) / elapsed),
							WriteOps: int64(float64(current.WriteCount-previous.WriteCount) / elapsed),
						}
					}
					diskIORatesMu.Lock()
					diskIORates = newRates
					diskIORatesMu.Unlock()
				}
			}
			if err == nil {
				lastCounters = counters
				lastSample = now
			}
			time.Sleep(1 * time.Second)
		}
	}()
}

// HandleDiskIORates serves the latest per-disk IO rates as JSON.
func HandleDiskIORates(w http.ResponseWriter, r *http.Request) {
	diskIORatesMu.RLock()
	rates := make([]*DiskIORate, 0, len(diskIORates))
	for _, rate := range diskIORates {
		rates = append(rates, rate)
	}
	diskIORatesMu.RUnlock()
	js, _ := json.Marshal(rates)
	utils.SendJSONResponse(w, string(js))
}
