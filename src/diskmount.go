package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"imuslab.com/bokodm/bokodmd/mod/diskinfo"
	"imuslab.com/bokodm/bokodmd/mod/disktool/diskfs"
	"imuslab.com/bokodm/bokodmd/mod/utils"
)

/*
	diskmount.go

	This file handles mounting local partitions onto host paths and the
	server-side folder browser used by the mount location picker.

	Support APIs

	/disks/mount   - POST partition=sda1, mountPoint=/media/data
	/disks/unmount - POST partition=sda1
	/disks/browse  - GET  dir=/media  → subdirectories of the given path
	/disks/mkdir   - POST dir=/media/newfolder
*/

// Paths that must never be used as a mount point or get folders created in
// by the web UI folder picker.
var protectedMountPrefixes = []string{
	"/proc", "/sys", "/dev", "/run", "/boot",
}

func mountPathIsProtected(path string) bool {
	cleaned := filepath.Clean(path)
	if cleaned == "/" {
		return true
	}
	for _, prefix := range protectedMountPrefixes {
		if cleaned == prefix || strings.HasPrefix(cleaned, prefix+"/") {
			return true
		}
	}
	return false
}

func HandleDiskMountCalls() http.Handler {
	return http.StripPrefix("/disks/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pathParts := strings.Split(r.URL.Path, "/")
		switch pathParts[0] {
		case "mount":
			handleMountPartition(w, r)
		case "unmount":
			handleUnmountPartition(w, r)
		case "browse":
			handleBrowseDirectory(w, r)
		case "mkdir":
			handleCreateDirectory(w, r)
		default:
			http.Error(w, "Not Found", http.StatusNotFound)
		}
	}))
}

// handleMountPartition mounts a partition (e.g. sda1) to the given path
func handleMountPartition(w http.ResponseWriter, r *http.Request) {
	partition, err := utils.PostPara(r, "partition")
	if err != nil {
		utils.SendErrorResponse(w, "partition not given")
		return
	}
	partition = filepath.Base(partition) // accept both sda1 and /dev/sda1

	mountPoint, err := utils.PostPara(r, "mountPoint")
	if err != nil {
		utils.SendErrorResponse(w, "mount point not given")
		return
	}

	if !strings.HasPrefix(mountPoint, "/") {
		utils.SendErrorResponse(w, "mount point must be an absolute path")
		return
	}

	if mountPathIsProtected(mountPoint) {
		utils.SendErrorResponse(w, "target path cannot be used as a mount point")
		return
	}

	if !diskinfo.DevicePathIsValidPartition(partition) {
		utils.SendErrorResponse(w, "invalid partition name")
		return
	}

	devicePath := filepath.Join("/dev/", partition)
	mounted, err := diskfs.DeviceIsMounted(devicePath)
	if err != nil {
		utils.SendErrorResponse(w, "unable to read device state")
		return
	}
	if mounted {
		utils.SendErrorResponse(w, "partition is already mounted")
		return
	}

	// Some filesystems need a userspace helper that the kernel mount
	// cannot provide on its own. Resolve the fs type first so we can
	// pick the right driver and give a useful install hint instead of
	// the raw "unknown filesystem type" error.
	mountType := ""
	partInfo, err := diskinfo.GetPartitionInfo(partition)
	if err == nil && partInfo.FsType == "ntfs" {
		if commandExists("mount.ntfs-3g") || commandExists("ntfs-3g") {
			mountType = "ntfs-3g"
		} else if commandExists("mount.ntfs3") {
			// Newer kernels ship the native ntfs3 driver
			mountType = "ntfs3"
		} else {
			utils.SendErrorResponse(w, "mounting NTFS requires the ntfs-3g package. Install it with: sudo apt-get install -y ntfs-3g")
			return
		}
	}

	// Create the mount point if it does not exist
	if err := os.MkdirAll(mountPoint, 0755); err != nil {
		utils.SendErrorResponse(w, "unable to create mount point: "+err.Error())
		return
	}

	// Refuse to shadow a non-empty directory: mounting over user data is
	// almost always a mistake
	entries, err := os.ReadDir(mountPoint)
	if err != nil {
		utils.SendErrorResponse(w, "unable to access mount point: "+err.Error())
		return
	}
	if len(entries) > 0 {
		utils.SendErrorResponse(w, "mount point directory is not empty")
		return
	}

	mountArgs := []string{"mount"}
	if mountType != "" {
		mountArgs = append(mountArgs, "-t", mountType)
	}
	mountArgs = append(mountArgs, devicePath, mountPoint)
	cmd := exec.Command("sudo", mountArgs...)
	output, err := utils.RunAndStream(cmd)
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if msg == "" {
			msg = err.Error()
		}
		utils.SendErrorResponse(w, "mount failed: "+msg)
		return
	}

	log.Println("[Disks] Partition " + devicePath + " mounted on " + mountPoint)
	utils.SendOK(w)
}

// handleUnmountPartition unmounts a mounted partition (e.g. sda1)
func handleUnmountPartition(w http.ResponseWriter, r *http.Request) {
	partition, err := utils.PostPara(r, "partition")
	if err != nil {
		utils.SendErrorResponse(w, "partition not given")
		return
	}
	partition = filepath.Base(partition)

	if !diskinfo.DevicePathIsValidPartition(partition) {
		utils.SendErrorResponse(w, "invalid partition name")
		return
	}

	// Never unmount system critical partitions
	partInfo, err := diskinfo.GetPartitionInfo(partition)
	if err != nil {
		utils.SendErrorResponse(w, "unable to read partition info")
		return
	}
	if partInfo.MountPoint == "/" || strings.HasPrefix(partInfo.MountPoint, "/boot") || partInfo.MountPoint == "[SWAP]" {
		utils.SendErrorResponse(w, "system partition cannot be unmounted")
		return
	}

	devicePath := filepath.Join("/dev/", partition)
	mounted, err := diskfs.DeviceIsMounted(devicePath)
	if err != nil {
		utils.SendErrorResponse(w, "unable to read device state")
		return
	}
	if !mounted {
		utils.SendErrorResponse(w, "partition is not mounted")
		return
	}

	if err := diskfs.UnmountDevice(devicePath); err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	log.Println("[Disks] Partition " + devicePath + " unmounted")
	utils.SendOK(w)
}

// DirectoryEntry is one folder inside the browsed directory.
type DirectoryEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// BrowseResult is the response of the browse API.
type BrowseResult struct {
	CurrentDir string           `json:"currentDir"`
	ParentDir  string           `json:"parentDir"`
	Folders    []DirectoryEntry `json:"folders"`
}

// handleBrowseDirectory lists the subdirectories of a given host path so the
// frontend can render a folder picker.
func handleBrowseDirectory(w http.ResponseWriter, r *http.Request) {
	dir, err := utils.GetPara(r, "dir")
	if err != nil || dir == "" {
		dir = "/"
	}

	dir = filepath.Clean(dir)
	if !strings.HasPrefix(dir, "/") {
		utils.SendErrorResponse(w, "directory must be an absolute path")
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		utils.SendErrorResponse(w, "unable to read directory: "+err.Error())
		return
	}

	folders := []DirectoryEntry{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// Skip hidden and virtual kernel filesystems at root level
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if dir == "/" && (entry.Name() == "proc" || entry.Name() == "sys" || entry.Name() == "dev" || entry.Name() == "run") {
			continue
		}
		folders = append(folders, DirectoryEntry{
			Name: entry.Name(),
			Path: filepath.Join(dir, entry.Name()),
		})
	}
	sort.Slice(folders, func(i, j int) bool {
		return folders[i].Name < folders[j].Name
	})

	parent := filepath.Dir(dir)
	result := BrowseResult{
		CurrentDir: dir,
		ParentDir:  parent,
		Folders:    folders,
	}

	js, _ := json.Marshal(result)
	utils.SendJSONResponse(w, string(js))
}

// handleCreateDirectory creates a new folder for use as a mount point.
func handleCreateDirectory(w http.ResponseWriter, r *http.Request) {
	dir, err := utils.PostPara(r, "dir")
	if err != nil {
		utils.SendErrorResponse(w, "directory not given")
		return
	}

	dir = filepath.Clean(dir)
	if !strings.HasPrefix(dir, "/") {
		utils.SendErrorResponse(w, "directory must be an absolute path")
		return
	}
	if mountPathIsProtected(dir) {
		utils.SendErrorResponse(w, "cannot create folder at this location")
		return
	}
	if utils.FileExists(dir) {
		utils.SendErrorResponse(w, "target already exists")
		return
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		utils.SendErrorResponse(w, fmt.Sprintf("unable to create folder: %v", err))
		return
	}

	utils.SendOK(w)
}
