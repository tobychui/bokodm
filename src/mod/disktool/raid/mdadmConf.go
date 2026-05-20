package raid

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"imuslab.com/bokofs/bokofsd/mod/disktool/diskfs"
	"imuslab.com/bokofs/bokofsd/mod/utils"
)

/*
	mdadmConf.go

	This package handles the config modification and update for
	the mdadm module
*/

// MdadmConfPath is the path to the mdadm configuration file.
// Update this constant if your system stores the config elsewhere.
const MdadmConfPath = "/etc/mdadm/mdadm.conf"

// Force mdadm to stop all RAID and load fresh from config file
// on some Linux distro this is required as mdadm start too early
func (m *Manager) FlushReload() error {
	//Get a list of currently running RAID devices
	raidDevices, err := m.GetRAIDDevicesFromProcMDStat()
	if err != nil {
		return err
	}

	//Stop all of the running RAID devices
	for _, rd := range raidDevices {
		err = m.FlushReloadDev(&rd)
		if err != nil {
			log.Println("[RAID] Unable to stop " + rd.Name + ": " + err.Error())
			continue
		}
	}

	time.Sleep(300 * time.Millisecond)

	//Assemble mdadm array again
	err = m.RestartRAIDService()
	if err != nil {
		return err
	}

	return nil
}

// FlushReloadDev stop a single RAID device and remove it from mdadm config
func (m *Manager) FlushReloadDev(targetDev *RAIDDevice) error {
	//Check if it is mounted. If yes, skip this
	devMounted, err := diskfs.DeviceIsMounted("/dev/" + targetDev.Name)
	if devMounted || err != nil {
		return errors.New("device is in use")
	}

	cmdMdadm := exec.Command("mdadm", "--stop", "/dev/"+targetDev.Name)

	// Run the command and capture its output
	_, err = cmdMdadm.Output()
	if err != nil {
		return err
	}
	return nil
}

// removeDevicesEntry remove device hardcode from mdadm config file
func removeDevicesEntry(configLine string) string {
	// Split the config line by space character
	tokens := strings.Fields(configLine)

	// Iterate through the tokens to find and remove the devices=* part
	for i, token := range tokens {
		if strings.HasPrefix(token, "devices=") {
			// Remove the devices=* part from the slice
			tokens = append(tokens[:i], tokens[i+1:]...)
			break
		}
	}

	// Join the tokens back into a single string
	updatedConfigLine := strings.Join(tokens, " ")

	return updatedConfigLine
}

// Updates the mdadm configuration file with the details of RAID arrays
// so the RAID drive will still be seen after a reboot (hopefully)
// this will automatically add / remove config base on current runtime setup
func (m *Manager) UpdateMDADMConfig() error {
	cmdMdadm := exec.Command("mdadm", "--detail", "--scan", "--verbose")

	// Run the command and capture its output
	output, err := cmdMdadm.Output()
	if err != nil {
		return fmt.Errorf("error running mdadm command: %v", err)
	}

	//Load the config from system
	currentConfigBytes, err := os.ReadFile(MdadmConfPath)
	if err != nil {
		return fmt.Errorf("unable to open mdadm.conf: " + err.Error())
	}
	currentConf := string(currentConfigBytes)

	//Check if the current config already contains the setting
	newConfigLines := []string{}
	uuidsInNewConfig := []string{}
	arrayConfigs := strings.TrimSpace(string(output))
	lines := strings.Split(arrayConfigs, "ARRAY")
	for _, line := range lines {
		//For each line, you should have something like this
		//ARRAY /dev/md0 metadata=1.2 name=debian:0 UUID=cbc11a2b:fbd42653:99c1340b:9c4962fb
		//   devices=/dev/sdb,/dev/sdc
		//Building structure for RAID Config Record

		line = strings.ReplaceAll(line, "\n", " ")
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		poolUUID := strings.TrimPrefix(fields[3], "UUID=")
		uuidsInNewConfig = append(uuidsInNewConfig, poolUUID)
		//Check if this uuid already in the config file
		if strings.Contains(currentConf, poolUUID) {
			continue
		}

		//This config not exists in the settings. Add it to append lines
		log.Println("[RAID] Adding " + fields[0] + " (UUID=" + poolUUID + ") into mdadm config")
		settingLine := "ARRAY " + strings.Join(fields, " ")

		//Remove the device specific names
		settingLine = removeDevicesEntry(settingLine)
		newConfigLines = append(newConfigLines, settingLine)
	}

	originalConfigLines := strings.Split(strings.TrimSpace(currentConf), "\n")
	poolUUIDToBeRemoved := []string{}
	for _, line := range originalConfigLines {
		lineFields := strings.Fields(line)
		for _, thisField := range lineFields {
			if strings.HasPrefix(thisField, "UUID=") {
				//This is the UUID of this array. Check if it still exists in new storage config
				thisPoolUUID := strings.TrimPrefix(thisField, "UUID=")
				existsInNewConfig := utils.StringInArray(uuidsInNewConfig, thisPoolUUID)
				if !existsInNewConfig {
					//Label this UUID to be removed
					poolUUIDToBeRemoved = append(poolUUIDToBeRemoved, thisPoolUUID)
				}

				//Skip scanning the remaining fields of this RAID pool
				break
			}
		}
	}

	if len(poolUUIDToBeRemoved) > 0 {
		//Remove the old UUIDs from config
		for _, volumeUUID := range poolUUIDToBeRemoved {
			err = m.RemoveVolumeFromMDADMConfig(volumeUUID)
			if err != nil {
				log.Println("[RAID] Error when trying to remove old RAID volume from config: " + err.Error())
				return err
			} else {
				log.Println("[RAID] RAID volume " + volumeUUID + " removed from config file")
			}
		}

	}

	if len(newConfigLines) == 0 {
		//Nothing to write
		log.Println("[RAID] Nothing to write. Skipping mdadm config update.")
		return nil
	}

	// Append new config lines directly to mdadm.conf using Go file I/O
	f, err := os.OpenFile(MdadmConfPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("error opening mdadm.conf for writing: %v", err)
	}
	defer f.Close()
	for _, configLine := range newConfigLines {
		if _, err := fmt.Fprintln(f, configLine); err != nil {
			return fmt.Errorf("error injecting line into mdadm.conf: %v", err)
		}
	}

	return nil
}

// Removes a RAID volume from the mdadm configuration file given its volume UUID.
// Note that this only remove a single line of config. If your line consists of multiple lines
// you might need to remove it manually
func (m *Manager) RemoveVolumeFromMDADMConfig(volumeUUID string) error {
	// Read the current config
	data, err := os.ReadFile(MdadmConfPath)
	if err != nil {
		return fmt.Errorf("error reading mdadm.conf: %v", err)
	}

	// Filter out any line containing the target UUID
	lines := strings.Split(string(data), "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		if !strings.Contains(line, "UUID="+volumeUUID) {
			filtered = append(filtered, line)
		}
	}

	// Write the filtered content back
	if err := os.WriteFile(MdadmConfPath, []byte(strings.Join(filtered, "\n")), 0644); err != nil {
		return fmt.Errorf("error writing mdadm.conf: %v", err)
	}

	return nil
}
