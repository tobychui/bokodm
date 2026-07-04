package logger

/*
	logger.go

	Central log store for bokodm. Everything worth reviewing later lands
	here: output of system tools (mkfs, parted, mdadm, mount...), the
	scheduled SMART checks, webhook notifications and system events.

	Entries are appended to one file per month (bokodm-YYYY-MM.log), so
	rotation happens naturally at month boundaries. A background cleanup
	removes files that exceed the configured retention age or when the
	log folder grows past the configured total size.

	Log line format (continuation lines of multi-line entries are
	indented and do not start with "["):

	  [2026-07-03 10:12:44] [cmd] $ sudo mkfs.ext4 /dev/md0p1 (ok)
	      mke2fs 1.47.0 (5-Feb-2023)
	      ...
*/

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	logFolder = "./logs"
	mu        sync.Mutex

	// retention config, guarded by configMu
	configMu        sync.RWMutex
	retentionMonths = 6   // delete monthly logs older than this, 0 = keep forever
	maxTotalSizeMB  = 256 // purge oldest files when the folder exceeds this, 0 = unlimited

	cleanupOnce sync.Once

	validLogName = regexp.MustCompile(`^[a-zA-Z0-9_\-.]+\.log$`)
)

// LogFileInfo describes one log file for the Logs tab.
type LogFileInfo struct {
	Name      string `json:"name"`
	SizeBytes int64  `json:"sizeBytes"`
	ModTime   string `json:"modTime"`
	IsCurrent bool   `json:"isCurrent"` // the file receiving new entries
}

// Init sets the log folder and makes sure it exists.
func Init(folder string) error {
	mu.Lock()
	defer mu.Unlock()
	logFolder = folder
	return os.MkdirAll(folder, 0755)
}

// SetConfig updates the retention policy and triggers a cleanup pass.
func SetConfig(months int, maxSizeMB int) {
	configMu.Lock()
	retentionMonths = months
	maxTotalSizeMB = maxSizeMB
	configMu.Unlock()
	Cleanup()
}

// currentLogName returns the name of this month's log file.
func currentLogName() string {
	return "bokodm-" + time.Now().Format("2006-01") + ".log"
}

// Write appends one entry to the current monthly log file.
// Multi-line messages are stored with indented continuation lines.
func Write(category string, message string) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")

	lines := strings.Split(strings.TrimRight(message, "\n"), "\n")
	entry := fmt.Sprintf("[%s] [%s] %s\n", timestamp, category, lines[0])
	for _, line := range lines[1:] {
		entry += "    " + line + "\n"
	}

	mu.Lock()
	defer mu.Unlock()
	f, err := os.OpenFile(filepath.Join(logFolder, currentLogName()), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println("[Logger] Unable to open log file:", err)
		return
	}
	defer f.Close()
	f.WriteString(entry)
}

// LogCommand records an executed system command and its output.
func LogCommand(args []string, output string, runErr error) {
	status := "ok"
	if runErr != nil {
		status = "failed: " + runErr.Error()
	}
	message := "$ " + strings.Join(args, " ") + " (" + status + ")"
	output = strings.TrimSpace(output)
	if output != "" {
		message += "\n" + output
	}
	Write("cmd", message)
}

// ListLogFiles returns every log file in the folder, newest first.
func ListLogFiles() ([]LogFileInfo, error) {
	entries, err := os.ReadDir(logFolder)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []LogFileInfo{}, nil
		}
		return nil, err
	}

	current := currentLogName()
	files := []LogFileInfo{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, LogFileInfo{
			Name:      entry.Name(),
			SizeBytes: info.Size(),
			ModTime:   info.ModTime().Format("2006-01-02 15:04:05"),
			IsCurrent: entry.Name() == current,
		})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Name > files[j].Name
	})
	return files, nil
}

// validateName rejects log file names that could escape the log folder.
func validateName(name string) error {
	if !validLogName.MatchString(name) || name != filepath.Base(name) {
		return errors.New("invalid log file name")
	}
	return nil
}

// ReadLogFile returns the content of one log file, tail-capped to maxBytes
// (0 = whole file) so huge files cannot blow up the browser.
func ReadLogFile(name string, maxBytes int64) (content string, truncated bool, err error) {
	if err := validateName(name); err != nil {
		return "", false, err
	}
	path := filepath.Join(logFolder, name)
	buf, err := os.ReadFile(path)
	if err != nil {
		return "", false, errors.New("log file not found")
	}

	if maxBytes > 0 && int64(len(buf)) > maxBytes {
		buf = buf[int64(len(buf))-maxBytes:]
		// Drop the first partial line after cutting into the middle
		if idx := strings.IndexByte(string(buf), '\n'); idx >= 0 {
			buf = buf[idx+1:]
		}
		truncated = true
	}
	return string(buf), truncated, nil
}

// LogFilePath returns the absolute path of a validated log file, used for
// download serving.
func LogFilePath(name string) (string, error) {
	if err := validateName(name); err != nil {
		return "", err
	}
	path := filepath.Join(logFolder, name)
	if _, err := os.Stat(path); err != nil {
		return "", errors.New("log file not found")
	}
	return path, nil
}

// DeleteLogFile removes one log file. The current month's file is protected.
func DeleteLogFile(name string) error {
	if err := validateName(name); err != nil {
		return err
	}
	if name == currentLogName() {
		return errors.New("the active log file cannot be deleted")
	}
	return os.Remove(filepath.Join(logFolder, name))
}

// Cleanup enforces the retention policy: drop files older than the
// configured number of months, then purge oldest files while the folder
// exceeds the configured total size. The current month's file survives.
func Cleanup() {
	configMu.RLock()
	months := retentionMonths
	maxMB := maxTotalSizeMB
	configMu.RUnlock()

	entries, err := os.ReadDir(logFolder)
	if err != nil {
		return
	}

	type fileMeta struct {
		name    string
		size    int64
		modTime time.Time
	}
	current := currentLogName()
	files := []fileMeta{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".log") || entry.Name() == current {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, fileMeta{entry.Name(), info.Size(), info.ModTime()})
	}

	// Age-based deletion
	if months > 0 {
		cutoff := time.Now().AddDate(0, -months, 0)
		kept := files[:0]
		for _, file := range files {
			if file.modTime.Before(cutoff) {
				os.Remove(filepath.Join(logFolder, file.name))
				fmt.Println("[Logger] Removed expired log file " + file.name)
				continue
			}
			kept = append(kept, file)
		}
		files = kept
	}

	// Size-based purge, oldest first
	if maxMB > 0 {
		var total int64
		for _, file := range files {
			total += file.size
		}
		if currentInfo, err := os.Stat(filepath.Join(logFolder, current)); err == nil {
			total += currentInfo.Size()
		}
		sort.Slice(files, func(i, j int) bool {
			return files[i].modTime.Before(files[j].modTime)
		})
		limit := int64(maxMB) * 1024 * 1024
		for _, file := range files {
			if total <= limit {
				break
			}
			if err := os.Remove(filepath.Join(logFolder, file.name)); err == nil {
				total -= file.size
				fmt.Println("[Logger] Removed log file " + file.name + " to keep folder size under limit")
			}
		}
	}
}

// StartAutoCleanup launches a periodic cleanup pass. Safe to call once.
func StartAutoCleanup() {
	cleanupOnce.Do(func() {
		go func() {
			Cleanup()
			ticker := time.NewTicker(12 * time.Hour)
			defer ticker.Stop()
			for range ticker.C {
				Cleanup()
			}
		}()
	})
}
