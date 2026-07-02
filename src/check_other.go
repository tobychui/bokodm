//go:build !linux && !darwin
// +build !linux,!darwin

package main

import (
	"fmt"
	"runtime"
)

func checkRuntimeEnv() *DependencyReport {
	fmt.Printf("\033[33m⚠\033[0m  Platform '%s' has limited support. Only SMART monitoring may be available.\n", runtime.GOOS)

	deps := buildDependencyList()

	allRequired := true
	for i := range deps {
		deps[i].Found = commandExists(deps[i].Name)
		if !deps[i].Found && deps[i].Required {
			allRequired = false
		}
		printDepCheck(&deps[i])
	}

	return &DependencyReport{
		AllSatisfied: allRequired,
		DegradedMode: false,
		Deps:         deps,
	}
}
