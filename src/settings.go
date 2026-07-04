package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"imuslab.com/bokodm/bokodmd/mod/logger"
	"imuslab.com/bokodm/bokodmd/mod/utils"
)

/*
	settings.go

	Persistent system settings (config/settings.json) and their API.

	Sections
	  Disk Monitoring — which disks to SMART-check on a schedule and how often
	  Notification    — webhook alerting with severity threshold and rate limit
	  Auto Mounts     — partitions mounted automatically when bokodm starts
	                    (network filesystems use the automount flag on the
	                    netmount connection instead)

	Support APIs

	/settings/get               - GET the full settings object
	/settings/monitor           - POST enabled, disks (JSON array), intervalMinutes
	/settings/notification      - POST enabled, url, threshold, rateNumber, rateUnit
	/settings/notification/test - POST send a test webhook now
	/settings/automount/add     - POST partition=sdX1, mountPoint=/media/data
	/settings/automount/remove  - POST uuid=<fs uuid>
*/

// MonitorSettings controls the scheduled SMART checker.
type MonitorSettings struct {
	Enabled         bool     `json:"enabled"`
	Disks           []string `json:"disks"`           // device names, e.g. ["sda", "sdb"]
	IntervalMinutes int      `json:"intervalMinutes"` // check interval
}

// NotificationSettings controls webhook alerting.
type NotificationSettings struct {
	Enabled    bool   `json:"enabled"`
	WebhookURL string `json:"webhookURL"`
	Threshold  string `json:"threshold"`  // "warning" | "failing" | "critical"
	RateNumber int    `json:"rateNumber"` // notify at most once per RateNumber * RateUnit (per disk)
	RateUnit   string `json:"rateUnit"`   // "hour" | "day" | "week"
}

// LogSettings controls log rotation and automatic deletion.
type LogSettings struct {
	// RetentionMonths deletes monthly log files older than this many
	// months. 0 keeps logs forever.
	RetentionMonths int `json:"retentionMonths"`
	// MaxTotalSizeMB purges the oldest log files when the log folder
	// exceeds this size. 0 disables the size limit.
	MaxTotalSizeMB int `json:"maxTotalSizeMB"`
}

// AutoMountEntry is a partition mounted automatically at startup.
type AutoMountEntry struct {
	UUID       string `json:"uuid"`      // filesystem UUID (stable across reboots)
	Partition  string `json:"partition"` // device name at creation time, informational
	MountPoint string `json:"mountPoint"`
}

// SysSettings is the persisted settings tree.
type SysSettings struct {
	Monitor      MonitorSettings      `json:"monitor"`
	Notification NotificationSettings `json:"notification"`
	AutoMounts   []AutoMountEntry     `json:"automounts"`
	Logs         LogSettings          `json:"logs"`
}

var (
	sysSettings     *SysSettings
	sysSettingsMu   sync.RWMutex
	settingsFile    = "./config/settings.json"
	validRateUnits  = map[string]bool{"hour": true, "day": true, "week": true}
	validThresholds = map[string]bool{"warning": true, "failing": true, "critical": true}
)

func defaultSettings() *SysSettings {
	return &SysSettings{
		Monitor: MonitorSettings{
			Enabled:         false,
			Disks:           []string{},
			IntervalMinutes: 60,
		},
		Notification: NotificationSettings{
			Enabled:    false,
			WebhookURL: "",
			Threshold:  "failing",
			RateNumber: 1,
			RateUnit:   "day",
		},
		AutoMounts: []AutoMountEntry{},
		Logs: LogSettings{
			RetentionMonths: 6,
			MaxTotalSizeMB:  256,
		},
	}
}

// loadSettings reads config/settings.json, falling back to defaults.
func loadSettings() {
	sysSettingsMu.Lock()
	defer sysSettingsMu.Unlock()

	sysSettings = defaultSettings()
	content, err := os.ReadFile(settingsFile)
	if err != nil {
		return
	}
	if err := json.Unmarshal(content, sysSettings); err != nil {
		log.Println("[Settings] settings.json corrupted, using defaults:", err)
		sysSettings = defaultSettings()
	}

	// Older settings files have no logs section, fall back to defaults
	if sysSettings.Logs.RetentionMonths == 0 && sysSettings.Logs.MaxTotalSizeMB == 0 {
		if !bytesContainLogsSection(content) {
			sysSettings.Logs = defaultSettings().Logs
		}
	}
}

// bytesContainLogsSection reports whether the raw settings JSON already has
// a "logs" key, so an explicit {0, 0} (= unlimited) is preserved.
func bytesContainLogsSection(content []byte) bool {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(content, &probe); err != nil {
		return false
	}
	_, ok := probe["logs"]
	return ok
}

// saveSettingsLocked persists the settings. Caller must hold sysSettingsMu.
func saveSettingsLocked() error {
	js, err := json.MarshalIndent(sysSettings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(settingsFile, js, 0600)
}

// applyAutoMounts mounts every configured automount entry (best-effort),
// called once during startup.
func applyAutoMounts() {
	sysSettingsMu.RLock()
	entries := make([]AutoMountEntry, len(sysSettings.AutoMounts))
	copy(entries, sysSettings.AutoMounts)
	sysSettingsMu.RUnlock()

	for _, entry := range entries {
		if err := mountByUUID(entry.UUID, entry.MountPoint); err != nil {
			log.Printf("[Settings] Automount UUID=%s on %s failed: %v", entry.UUID, entry.MountPoint, err)
		} else {
			log.Printf("[Settings] Automounted UUID=%s on %s", entry.UUID, entry.MountPoint)
		}
	}
}

// mountByUUID mounts a filesystem by its UUID if it is not mounted yet.
func mountByUUID(uuid string, mountPoint string) error {
	if uuid == "" || mountPoint == "" {
		return errors.New("invalid automount entry")
	}

	// Already mounted at the target?
	mounts, err := os.ReadFile("/proc/mounts")
	if err == nil {
		for _, line := range strings.Split(string(mounts), "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 2 && fields[1] == mountPoint {
				return nil // something is already mounted there
			}
		}
	}

	if err := os.MkdirAll(mountPoint, 0755); err != nil {
		return err
	}

	cmd := exec.Command("sudo", "mount", "UUID="+uuid, mountPoint)
	output, err := utils.RunAndStream(cmd)
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if msg == "" {
			msg = err.Error()
		}
		return errors.New(msg)
	}
	return nil
}

func HandleSettingsCalls() http.Handler {
	return http.StripPrefix("/settings/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pathParts := strings.Split(r.URL.Path, "/")
		switch pathParts[0] {
		case "get":
			sysSettingsMu.RLock()
			js, _ := json.Marshal(sysSettings)
			sysSettingsMu.RUnlock()
			utils.SendJSONResponse(w, string(js))
		case "monitor":
			handleSetMonitorSettings(w, r)
		case "logs":
			handleSetLogSettings(w, r)
		case "notification":
			if len(pathParts) >= 2 && pathParts[1] == "test" {
				handleTestNotification(w, r)
				return
			}
			handleSetNotificationSettings(w, r)
		case "automount":
			if len(pathParts) < 2 {
				http.Error(w, "Bad Request", http.StatusBadRequest)
				return
			}
			switch pathParts[1] {
			case "add":
				handleAddAutoMount(w, r)
			case "remove":
				handleRemoveAutoMount(w, r)
			default:
				http.Error(w, "Not Found", http.StatusNotFound)
			}
		default:
			http.Error(w, "Not Found", http.StatusNotFound)
		}
	}))
}

func handleSetMonitorSettings(w http.ResponseWriter, r *http.Request) {
	enabled, err := utils.PostBool(r, "enabled")
	if err != nil {
		utils.SendErrorResponse(w, "enabled flag not given")
		return
	}

	disksJSON, err := utils.PostPara(r, "disks")
	if err != nil {
		utils.SendErrorResponse(w, "disks not given")
		return
	}
	disks := []string{}
	if err := json.Unmarshal([]byte(disksJSON), &disks); err != nil {
		utils.SendErrorResponse(w, "unable to parse disks array")
		return
	}
	for i := range disks {
		disks[i] = filepath.Base(disks[i])
	}

	interval, err := utils.PostInt(r, "intervalMinutes")
	if err != nil || interval < 1 {
		utils.SendErrorResponse(w, "invalid check interval")
		return
	}

	sysSettingsMu.Lock()
	sysSettings.Monitor.Enabled = enabled
	sysSettings.Monitor.Disks = disks
	sysSettings.Monitor.IntervalMinutes = interval
	err = saveSettingsLocked()
	sysSettingsMu.Unlock()
	if err != nil {
		utils.SendErrorResponse(w, "unable to save settings: "+err.Error())
		return
	}

	restartDiskMonitor()
	utils.SendOK(w)
}

// handleSetLogSettings updates the log rotation policy, require
// "retentionMonths" and "maxTotalSizeMB" (0 = unlimited for both).
func handleSetLogSettings(w http.ResponseWriter, r *http.Request) {
	retentionMonths, err := utils.PostInt(r, "retentionMonths")
	if err != nil || retentionMonths < 0 || retentionMonths > 1200 {
		utils.SendErrorResponse(w, "invalid retention months")
		return
	}
	maxTotalSizeMB, err := utils.PostInt(r, "maxTotalSizeMB")
	if err != nil || maxTotalSizeMB < 0 || maxTotalSizeMB > 1024*1024 {
		utils.SendErrorResponse(w, "invalid max total size")
		return
	}

	sysSettingsMu.Lock()
	sysSettings.Logs.RetentionMonths = retentionMonths
	sysSettings.Logs.MaxTotalSizeMB = maxTotalSizeMB
	err = saveSettingsLocked()
	sysSettingsMu.Unlock()
	if err != nil {
		utils.SendErrorResponse(w, "unable to save settings: "+err.Error())
		return
	}

	// Apply immediately, this also triggers a cleanup pass
	logger.SetConfig(retentionMonths, maxTotalSizeMB)
	utils.SendOK(w)
}

func handleSetNotificationSettings(w http.ResponseWriter, r *http.Request) {
	enabled, err := utils.PostBool(r, "enabled")
	if err != nil {
		utils.SendErrorResponse(w, "enabled flag not given")
		return
	}

	webhookURL, _ := utils.PostPara(r, "url")
	if enabled {
		parsed, err := url.Parse(webhookURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			utils.SendErrorResponse(w, "invalid webhook URL")
			return
		}
	}

	threshold, err := utils.PostPara(r, "threshold")
	if err != nil || !validThresholds[threshold] {
		utils.SendErrorResponse(w, "threshold must be warning, failing or critical")
		return
	}

	rateNumber, err := utils.PostInt(r, "rateNumber")
	if err != nil || rateNumber < 1 {
		utils.SendErrorResponse(w, "invalid rate number")
		return
	}
	rateUnit, err := utils.PostPara(r, "rateUnit")
	if err != nil || !validRateUnits[rateUnit] {
		utils.SendErrorResponse(w, "rate unit must be hour, day or week")
		return
	}

	sysSettingsMu.Lock()
	sysSettings.Notification.Enabled = enabled
	sysSettings.Notification.WebhookURL = webhookURL
	sysSettings.Notification.Threshold = threshold
	sysSettings.Notification.RateNumber = rateNumber
	sysSettings.Notification.RateUnit = rateUnit
	err = saveSettingsLocked()
	sysSettingsMu.Unlock()
	if err != nil {
		utils.SendErrorResponse(w, "unable to save settings: "+err.Error())
		return
	}

	utils.SendOK(w)
}

func handleTestNotification(w http.ResponseWriter, r *http.Request) {
	webhookURL, err := utils.PostPara(r, "url")
	if err != nil || webhookURL == "" {
		utils.SendErrorResponse(w, "webhook URL not given")
		return
	}

	err = sendWebhookNotification(webhookURL, "test", "bokodm test notification", nil)
	if err != nil {
		utils.SendErrorResponse(w, "webhook delivery failed: "+err.Error())
		return
	}
	utils.SendOK(w)
}

func handleAddAutoMount(w http.ResponseWriter, r *http.Request) {
	partition, err := utils.PostPara(r, "partition")
	if err != nil {
		utils.SendErrorResponse(w, "partition not given")
		return
	}
	partition = filepath.Base(partition)

	mountPoint, err := utils.PostPara(r, "mountPoint")
	if err != nil || !strings.HasPrefix(mountPoint, "/") {
		utils.SendErrorResponse(w, "mount point must be an absolute path")
		return
	}
	if mountPathIsProtected(mountPoint) {
		utils.SendErrorResponse(w, "target path cannot be used as a mount point")
		return
	}

	// Resolve the filesystem UUID so the entry survives device renames
	cmd := exec.Command("sudo", "blkid", "-s", "UUID", "-o", "value", "/dev/"+partition)
	output, err := cmd.CombinedOutput()
	uuid := strings.TrimSpace(string(output))
	if err != nil || uuid == "" {
		utils.SendErrorResponse(w, "unable to resolve filesystem UUID (is the partition formatted?)")
		return
	}

	sysSettingsMu.Lock()
	for _, entry := range sysSettings.AutoMounts {
		if entry.UUID == uuid {
			sysSettingsMu.Unlock()
			utils.SendErrorResponse(w, "an automount entry for this partition already exists")
			return
		}
	}
	sysSettings.AutoMounts = append(sysSettings.AutoMounts, AutoMountEntry{
		UUID:       uuid,
		Partition:  partition,
		MountPoint: mountPoint,
	})
	err = saveSettingsLocked()
	sysSettingsMu.Unlock()
	if err != nil {
		utils.SendErrorResponse(w, "unable to save settings: "+err.Error())
		return
	}

	// Mount it right away so the user gets immediate feedback
	if err := mountByUUID(uuid, mountPoint); err != nil {
		utils.SendErrorResponse(w, "entry saved but mount failed: "+err.Error())
		return
	}

	utils.SendOK(w)
}

func handleRemoveAutoMount(w http.ResponseWriter, r *http.Request) {
	uuid, err := utils.PostPara(r, "uuid")
	if err != nil {
		utils.SendErrorResponse(w, "uuid not given")
		return
	}

	sysSettingsMu.Lock()
	newList := []AutoMountEntry{}
	found := false
	for _, entry := range sysSettings.AutoMounts {
		if entry.UUID == uuid {
			found = true
			continue
		}
		newList = append(newList, entry)
	}
	sysSettings.AutoMounts = newList
	err = saveSettingsLocked()
	sysSettingsMu.Unlock()

	if !found {
		utils.SendErrorResponse(w, "automount entry not found")
		return
	}
	if err != nil {
		utils.SendErrorResponse(w, "unable to save settings: "+err.Error())
		return
	}
	// The entry only controls startup mounting; the filesystem stays
	// mounted until the user unmounts it in the Disks tab
	utils.SendOK(w)
}

// notifyRateWindow converts the rate settings into a duration string for
// logging, e.g. "1 day".
func notifyRateWindow(n NotificationSettings) string {
	return fmt.Sprintf("%d %s", n.RateNumber, n.RateUnit)
}
