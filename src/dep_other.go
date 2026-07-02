//go:build !linux && !darwin
// +build !linux,!darwin

package main

// buildDependencyList returns a minimal stub list for unsupported platforms.
// Only smartctl is checked; everything else is unavailable.
func buildDependencyList() []Dependency {
	return []Dependency{
		{
			Name:         "smartctl",
			Required:     false,
			Feature:      "smart",
			Description:  "SMART disk health monitoring",
			InstallHints: nil,
		},
	}
}
