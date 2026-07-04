//go:build linux
// +build linux

package main

import "fmt"

// ---- package-manager detection ----

// linuxPkgManagers is checked in priority order; first found wins.
var linuxPkgManagers = []struct {
	bin     string
	install string // printf template: install <pkg>
}{
	{"apt-get", "sudo apt-get install -y %s"},
	{"dnf", "sudo dnf install -y %s"},
	{"yum", "sudo yum install -y %s"},
	{"pacman", "sudo pacman -S %s"},
	{"zypper", "sudo zypper install %s"},
	{"apk", "sudo apk add %s"},
}

// linuxPackageNames maps a binary name → its package name for each manager.
// Keys that are absent for a given manager mean that manager cannot install it.
var linuxPackageNames = map[string]string{
	// binary  : package (same name for all managers listed above)
	"smartctl": "smartmontools",
	"mdadm":    "mdadm",
	"lsblk":    "util-linux",
	"blkid":    "util-linux",
	"df":       "coreutils",
	// Filesystem drivers (optional)
	"ntfs-3g": "ntfs-3g",
	// Partitioning (optional)
	"parted": "parted",
	// Network filesystem mount helpers (all optional)
	"mount.davfs": "davfs2",
	"curlftpfs":   "curlftpfs",
	"mount.cifs":  "cifs-utils",
	"mount.nfs":   "nfs-common",
}

func detectLinuxPackageManager() (binTemplate string, found bool) {
	for _, pm := range linuxPkgManagers {
		if commandExists(pm.bin) {
			return pm.install, true
		}
	}
	return "", false
}

func makeLinuxHints(cmdName string) []string {
	pkgName, ok := linuxPackageNames[cmdName]
	if !ok {
		return nil
	}
	tmpl, found := detectLinuxPackageManager()
	if !found {
		return nil
	}
	return []string{fmt.Sprintf(tmpl, pkgName)}
}

// ---- dependency list ----

func buildDependencyList() []Dependency {
	return []Dependency{
		{
			Name:         "smartctl",
			Required:     false,
			Feature:      "smart",
			Description:  "SMART disk health monitoring",
			InstallHints: makeLinuxHints("smartctl"),
		},
		{
			Name:         "mdadm",
			Required:     false,
			Feature:      "raid",
			Description:  "Software RAID management",
			InstallHints: makeLinuxHints("mdadm"),
		},
		{
			Name:         "lsblk",
			Required:     true,
			Feature:      "disks",
			Description:  "Block device enumeration",
			InstallHints: makeLinuxHints("lsblk"),
		},
		{
			Name:         "blkid",
			Required:     true,
			Feature:      "disks",
			Description:  "Partition UUID / filesystem type",
			InstallHints: makeLinuxHints("blkid"),
		},
		{
			Name:         "df",
			Required:     true,
			Feature:      "disks",
			Description:  "Disk usage statistics",
			InstallHints: makeLinuxHints("df"),
		},
		{
			Name:         "ntfs-3g",
			Required:     false,
			Feature:      "ntfs",
			Description:  "NTFS filesystem mounting / formatting",
			InstallHints: makeLinuxHints("ntfs-3g"),
		},
		{
			Name:         "parted",
			Required:     false,
			Feature:      "parttool",
			Description:  "Disk partitioning and partition table editing",
			InstallHints: makeLinuxHints("parted"),
		},
		{
			Name:         "mount.davfs",
			Required:     false,
			Feature:      "netmount",
			Description:  "WebDAV network filesystem mounting",
			InstallHints: makeLinuxHints("mount.davfs"),
		},
		{
			Name:         "curlftpfs",
			Required:     false,
			Feature:      "netmount",
			Description:  "FTP network filesystem mounting",
			InstallHints: makeLinuxHints("curlftpfs"),
		},
		{
			Name:         "mount.cifs",
			Required:     false,
			Feature:      "netmount",
			Description:  "SMB / CIFS network filesystem mounting",
			InstallHints: makeLinuxHints("mount.cifs"),
		},
		{
			Name:         "mount.nfs",
			Required:     false,
			Feature:      "netmount",
			Description:  "NFS network filesystem mounting",
			InstallHints: makeLinuxHints("mount.nfs"),
		},
	}
}
