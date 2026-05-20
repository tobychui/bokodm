package raid

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

/*
	RAID management package for handling RAID and Virtual Image Creation
	for Linux with mdadm installed
*/

type Manager struct {
}

// Create a new raid manager
func NewRaidManager() (*Manager, error) {
	//Check if platform is supported
	if runtime.GOOS != "linux" {
		return nil, errors.New("ArozOS do not support RAID management on this platform")
	}

	//Check if mdadm exists in PATH
	_, err := exec.LookPath("mdadm")
	if err != nil {
		return nil, errors.New("mdadm not found in PATH. Is it installed?")
	}
	return &Manager{}, nil
}

// Create a virtual image partition at given path with given size
func CreateVirtualPartition(imagePath string, totalSize int64) error {
	cmd := exec.Command("dd", "if=/dev/zero", "of="+imagePath, "bs=4M", "count="+fmt.Sprintf("%dM", totalSize/(4*1024*1024)))

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

	cmd := exec.Command("mkfs.ext4", imagePath)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("error running mkfs.ext4 command: %v", err)
	}

	return nil
}
