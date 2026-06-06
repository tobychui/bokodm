package raid

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

/*
	RAID management package for handling RAID and Virtual Image Creation
	for Linux with mdadm installed
*/

type Manager struct {
}

func PackageExists(packageName string) (bool, error) {
	cmd := exec.Command("dpkg-query", "-W", "-f='${Status}'", packageName)
	output, err := cmd.Output()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok && exitError.ExitCode() == 1 {
			// Package not found
			return false, nil
		}
		return false, fmt.Errorf("error checking package: %v", err)
	}

	// Check if the output contains "install ok installed"
	return string(output) == "'install ok installed'", nil
}

// Create a new raid manager
func NewRaidManager() (*Manager, error) {
	//Check if mdadm exists
	mdadmExists, err := PackageExists("mdadm")
	if err != nil || !mdadmExists {
		return nil, errors.New("mdadm not installed on this host")
	}
	return &Manager{}, nil
}

// Create a virtual image partition at given path with given size
func CreateVirtualPartition(imagePath string, totalSize int64) error {
	cmd := exec.Command("sudo", "dd", "if=/dev/zero", "of="+imagePath, "bs=4M", "count="+fmt.Sprintf("%dM", totalSize/(4*1024*1024)))

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("dd error: %v", err)
	}

	return nil
}

// Format the given image file
func FormatVirtualPartition(imagePath string) error {
	//Check if image actually exists
	if _, err := os.Stat(imagePath); os.IsNotExist(err) {
		return errors.New("image file does not exist")
	}

	if filepath.Ext(imagePath) != ".img" {
		return errors.New("given file is not an image path")
	}

	cmd := exec.Command("sudo", "mkfs.ext4", imagePath)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("error running mkfs.ext4 command: %v", err)
	}

	return nil
}
