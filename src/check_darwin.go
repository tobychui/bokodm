//go:build darwin
// +build darwin

package main

import "fmt"

// checkRuntimeEnv verifies required commands on macOS.
// mdadm / lsblk / blkid are Linux-only; diskutil is always built into macOS.
func checkRuntimeEnv() *DependencyReport {
	deps := buildDependencyList()

	allRequired := true
	for i := range deps {
		deps[i].Found = commandExists(deps[i].Name)
		if !deps[i].Found && deps[i].Required {
			allRequired = false
		}
		printDepCheck(&deps[i])
	}

	// RAID is Linux-only — mention it explicitly so users aren't surprised.
	fmt.Println("\033[33m⚠\033[0m  RAID management (mdadm) is not available on macOS")

	if allRequired {
		fmt.Println("\033[32mAll required dependencies are satisfied.\033[0m")
	} else {
		fmt.Println("\033[31mOne or more required dependencies are missing.\033[0m")
	}

	return &DependencyReport{
		AllSatisfied: allRequired,
		DegradedMode: false,
		Deps:         deps,
	}
}
