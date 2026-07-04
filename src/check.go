package main

/*
	check.go

	Runtime environment validation.
	check_linux.go   — verifies commands required on Linux
	check_darwin.go  — verifies commands required on macOS
	check_other.go   — stub for unsupported platforms

	buildDependencyList() is platform-specific and lives in dep_*.go.
*/

import (
	"os"
	"os/exec"
	"path/filepath"
)

// commandExists reports whether cmd is found on the PATH or in one of the
// standard sbin locations (mount helpers like mount.cifs live in /sbin,
// which is not always on PATH when running without a login shell).
func commandExists(cmd string) bool {
	if _, err := exec.LookPath(cmd); err == nil {
		return true
	}
	for _, dir := range []string{"/sbin", "/usr/sbin", "/usr/local/sbin"} {
		if _, err := os.Stat(filepath.Join(dir, cmd)); err == nil {
			return true
		}
	}
	return false
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
