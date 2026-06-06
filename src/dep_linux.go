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
	}
}
