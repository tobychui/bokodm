package main

/*
	check.go

	Runtime environment validation.
	check_linux.go   — verifies commands required on Linux
	check_darwin.go  — verifies commands required on macOS
	check_other.go   — stub for unsupported platforms

	buildDependencyList() is platform-specific and lives in dep_*.go.
*/

import "os/exec"

// commandExists reports whether cmd is found on the PATH.
func commandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

// checkRuntimeEnvironment probes all platform-required external commands,
// prints coloured status lines (with install hints for missing deps), and
// returns a DependencyReport that can be served to the frontend.
//
// DegradedMode is NOT set here — the caller (start.go) sets it based on
// the -skip_dep flag after reviewing AllSatisfied.
func checkRuntimeEnvironment() *DependencyReport {
	return checkRuntimeEnv()
}
