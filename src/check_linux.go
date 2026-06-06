//go:build linux
// +build linux

package main

import "fmt"

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

	if allRequired {
		fmt.Println("\033[32mAll required dependencies are satisfied.\033[0m")
	} else {
		fmt.Println("\033[31mOne or more required dependencies are missing.\033[0m")
	}

	return &DependencyReport{
		AllSatisfied: allRequired,
		DegradedMode: false, // set by caller based on -skip_dep
		Deps:         deps,
	}
}
