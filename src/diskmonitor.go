package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"imuslab.com/bokodm/bokodmd/mod/diskinfo/smart"
	"imuslab.com/bokodm/bokodmd/mod/logger"
)

/*
	diskmonitor.go

	Scheduled SMART health checker. When enabled in the settings, the
	selected disks are checked at the configured interval and the result is
	recorded in the monthly log store (Logs tab, category "smart").

	When webhook notification is enabled and a disk reaches the configured
	severity threshold, a JSON payload is POSTed to the webhook URL. Per
	disk, notifications are rate limited to once per configured window;
	the last-notified timestamps persist across restarts in
	config/notify_state.json.

	Severity levels
	  warning  — early indicators (reallocated / pending sectors, CRC errors)
	  failing  — SMART overall check failed or heavy reallocation
	  critical — the disk no longer answers SMART queries (IO error) or
	             reports uncorrectable errors
*/

const (
	diskLevelOK       = 0
	diskLevelWarning  = 1
	diskLevelFailing  = 2
	diskLevelCritical = 3
)

var diskLevelNames = map[int]string{
	diskLevelOK:       "ok",
	diskLevelWarning:  "warning",
	diskLevelFailing:  "failing",
	diskLevelCritical: "critical",
}

var diskLevelValues = map[string]int{
	"warning":  diskLevelWarning,
	"failing":  diskLevelFailing,
	"critical": diskLevelCritical,
}

var (
	diskMonitorStop  chan bool
	diskMonitorMu    sync.Mutex
	notifyStateFile  = "./config/notify_state.json"
	notifyState      = map[string]time.Time{} // disk name → last webhook time
	notifyStateMu    sync.Mutex
)

// classifyDiskHealth turns a SMART health summary into a severity level.
// A nil health (query failed → disk not answering, likely IO error)
// classifies as critical.
func classifyDiskHealth(health *smart.DriveHealthInfo) int {
	if health == nil {
		return diskLevelCritical
	}
	if health.UncorrectableErrors > 0 {
		return diskLevelCritical
	}
	if !health.IsHealthy || health.ReallocatedSectors > 30 || health.ReallocateNANDBlocks > 30 {
		return diskLevelFailing
	}
	if health.ReallocatedSectors > 10 || health.ReallocateNANDBlocks > 10 ||
		health.PendingSectors >= 1 || health.UDMACRCErrors >= 10 {
		return diskLevelWarning
	}
	return diskLevelOK
}

// appendMonitorLog records one SMART check result in the log store.
func appendMonitorLog(diskName string, level int, health *smart.DriveHealthInfo, checkErr error) {
	message := fmt.Sprintf("/dev/%s level=%s", diskName, diskLevelNames[level])
	if checkErr != nil {
		message += " error=" + strconv.Quote(checkErr.Error())
	}
	if health != nil {
		message += fmt.Sprintf(" model=%s serial=%s healthy=%t power_on_hours=%d reallocated=%d pending=%d uncorrectable=%d crc_errors=%d",
			strconv.Quote(health.DeviceModel), health.SerialNumber, health.IsHealthy,
			health.PowerOnHours, health.ReallocatedSectors, health.PendingSectors,
			health.UncorrectableErrors, health.UDMACRCErrors)
	}
	logger.Write("smart", message)
}

// loadNotifyState restores the per-disk last-notified timestamps.
func loadNotifyState() {
	content, err := os.ReadFile(notifyStateFile)
	if err != nil {
		return
	}
	notifyStateMu.Lock()
	defer notifyStateMu.Unlock()
	json.Unmarshal(content, &notifyState)
}

func saveNotifyState() {
	notifyStateMu.Lock()
	js, _ := json.Marshal(notifyState)
	notifyStateMu.Unlock()
	os.WriteFile(notifyStateFile, js, 0600)
}

// notifyRateDuration converts the configured rate limit into a duration.
func notifyRateDuration(n NotificationSettings) time.Duration {
	unit := time.Hour
	switch n.RateUnit {
	case "day":
		unit = 24 * time.Hour
	case "week":
		unit = 7 * 24 * time.Hour
	}
	if n.RateNumber < 1 {
		return unit
	}
	return time.Duration(n.RateNumber) * unit
}

// sendWebhookNotification POSTs a JSON alert to the webhook URL.
func sendWebhookNotification(webhookURL string, level string, message string, health *smart.DriveHealthInfo) error {
	hostname, _ := os.Hostname()
	payload := map[string]interface{}{
		"source":  "bokodm",
		"host":    hostname,
		"level":   level,
		"message": message,
		"time":    time.Now().Format(time.RFC3339),
	}
	if health != nil {
		payload["disk"] = health.DeviceName
		payload["model"] = health.DeviceModel
		payload["serial"] = health.SerialNumber
		payload["health"] = health
	}

	js, _ := json.Marshal(payload)
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Post(webhookURL, "application/json", bytes.NewReader(js))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook responded with status %d", resp.StatusCode)
	}
	return nil
}

// maybeNotify sends a webhook alert when the level reaches the configured
// threshold and the per-disk rate limit allows it.
func maybeNotify(diskName string, level int, health *smart.DriveHealthInfo) {
	sysSettingsMu.RLock()
	notification := sysSettings.Notification
	sysSettingsMu.RUnlock()

	if !notification.Enabled || notification.WebhookURL == "" {
		return
	}
	if level < diskLevelValues[notification.Threshold] {
		return
	}

	notifyStateMu.Lock()
	lastNotified, seen := notifyState[diskName]
	window := notifyRateDuration(notification)
	if seen && time.Since(lastNotified) < window {
		notifyStateMu.Unlock()
		return
	}
	notifyState[diskName] = time.Now()
	notifyStateMu.Unlock()
	saveNotifyState()

	message := fmt.Sprintf("Disk /dev/%s health level is %s", diskName, diskLevelNames[level])
	if err := sendWebhookNotification(notification.WebhookURL, diskLevelNames[level], message, health); err != nil {
		log.Println("[DiskMonitor] Webhook delivery failed:", err)
		logger.Write("notify", "webhook delivery FAILED for /dev/"+diskName+" ("+diskLevelNames[level]+"): "+err.Error())
	} else {
		log.Printf("[DiskMonitor] Webhook alert sent for /dev/%s (%s), next alert in %s", diskName, diskLevelNames[level], notifyRateWindow(notification))
		logger.Write("notify", "webhook alert sent for /dev/"+diskName+" ("+diskLevelNames[level]+"), next alert in "+notifyRateWindow(notification))
	}
}

// runDiskMonitorCheck checks every monitored disk once.
func runDiskMonitorCheck() {
	sysSettingsMu.RLock()
	disks := make([]string, len(sysSettings.Monitor.Disks))
	copy(disks, sysSettings.Monitor.Disks)
	sysSettingsMu.RUnlock()

	for _, diskName := range disks {
		health, err := smart.GetDiskSMARTHealthSummary(diskName)
		if err != nil {
			health = nil
		}
		level := classifyDiskHealth(health)
		appendMonitorLog(diskName, level, health, err)
		if level > diskLevelOK {
			maybeNotify(diskName, level, health)
		}
	}
}

// restartDiskMonitor (re)starts the scheduler based on current settings.
// Safe to call whenever the monitor settings change.
func restartDiskMonitor() {
	diskMonitorMu.Lock()
	defer diskMonitorMu.Unlock()

	// Stop the previous scheduler if any
	if diskMonitorStop != nil {
		close(diskMonitorStop)
		diskMonitorStop = nil
	}

	sysSettingsMu.RLock()
	enabled := sysSettings.Monitor.Enabled
	interval := sysSettings.Monitor.IntervalMinutes
	diskCount := len(sysSettings.Monitor.Disks)
	sysSettingsMu.RUnlock()

	if !enabled || interval < 1 || diskCount == 0 {
		log.Println("[DiskMonitor] Scheduled SMART monitoring disabled")
		return
	}

	stop := make(chan bool)
	diskMonitorStop = stop
	log.Printf("[DiskMonitor] Monitoring %d disk(s) every %d minute(s)", diskCount, interval)

	go func() {
		// Run the first check shortly after start so the user sees output
		// without waiting a full interval
		firstRun := time.After(10 * time.Second)
		ticker := time.NewTicker(time.Duration(interval) * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-firstRun:
				runDiskMonitorCheck()
			case <-ticker.C:
				runDiskMonitorCheck()
			}
		}
	}()
}
