package raid

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type SyncState struct {
	DeviceName      string  //e.g, md0
	ResyncPercent   float64 //e.g, 0.5
	CompletedBlocks int64
	TotalBlocks     int64
	ExpectedTime    string //e.g, 1h23m
	Speed           string //e.g, 1234K/s
}

// GetSyncStateByPath retrieves the synchronization state of a RAID device by its device path.
// devpath can be either a full path (e.g., /dev/md0) or just the device name (e.g., md0).
func (m *Manager) GetSyncStateByPath(devpath string) (*SyncState, error) {
	devpath = strings.TrimPrefix(devpath, "/dev/")

	syncStates, err := m.GetSyncStates()
	if err != nil {
		return nil, err
	}

	for _, syncState := range syncStates {
		if syncState.DeviceName == devpath {
			return &syncState, nil
		}
	}

	return nil, errors.New("device not found")
}

// GetSyncStates retrieves the synchronization states of RAID arrays from /proc/mdstat.
func (m *Manager) GetSyncStates() ([]SyncState, error) {
	file, err := os.Open("/proc/mdstat")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var syncStates []SyncState
	var lastDeviceName string = ""
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "resync =") || strings.Contains(line, "recovery =") {
			parts := strings.Fields(line)
			var syncState SyncState

			for i, part := range parts {
				if part == "resync" || part == "recovery" {
					// Extract percentage
					if i+2 < len(parts) && strings.HasSuffix(parts[i+2], "%") {
						fmt.Sscanf(parts[i+2], "%f%%", &syncState.ResyncPercent)
					}

					// Extract completed and total blocks
					if i+3 < len(parts) && strings.HasPrefix(parts[i+3], "(") && strings.Contains(parts[i+3], "/") {
						var completed, total int64
						fmt.Sscanf(parts[i+3], "(%d/%d)", &completed, &total)
						syncState.CompletedBlocks = completed
						syncState.TotalBlocks = total
					}

					// Extract expected time
					if i+4 < len(parts) && strings.HasPrefix(parts[i+4], "finish=") {
						syncState.ExpectedTime = strings.TrimPrefix(parts[i+4], "finish=")
					}

					// Extract speed
					if i+5 < len(parts) && strings.HasPrefix(parts[i+5], "speed=") {
						syncState.Speed = strings.TrimPrefix(parts[i+5], "speed=")
					}
				}
			}

			syncState.DeviceName = lastDeviceName
			if syncState.DeviceName == "" {
				return nil, errors.New("device name not found")
			}

			// Add the sync state to the list
			syncStates = append(syncStates, syncState)
		} else if strings.HasPrefix(line, "md") {
			// Extract device name
			parts := strings.Fields(line)
			if len(parts) > 0 {
				lastDeviceName = strings.TrimPrefix(parts[0], "/dev/")
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return syncStates, nil
}

// SetSyncPendingToReadWrite sets the RAID device to read-write mode.
// After a RAID array is created, it may be in a "sync-pending" state.
// This function changes the state to "read-write".
func (m *Manager) SetSyncPendingToReadWrite(devname string) error {
	// Ensure devname does not already have the /dev/ prefix
	devname = strings.TrimPrefix(devname, "/dev/")

	// Construct the command
	cmd := exec.Command("mdadm", "--readwrite", fmt.Sprintf("/dev/%s", devname))
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to set device %s to readwrite: %v, output: %s", devname, err, string(output))
	}
	return nil
}
