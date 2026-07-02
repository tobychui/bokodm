//go:build darwin
// +build darwin

package main

import "fmt"

// ---- macOS package-manager hints ----

// macOSPackageFormulas maps binary names → Homebrew formula names.
var macOSPackageFormulas = map[string]string{
	"smartctl": "smartmontools",
}

// macOSPortPackages maps binary names → MacPorts port names.
var macOSPortPackages = map[string]string{
	"smartctl": "smartmontools",
}

func makeDarwinHints(cmdName string) []string {
	var hints []string

	if formula, ok := macOSPackageFormulas[cmdName]; ok {
		if commandExists("brew") {
			hints = append(hints, fmt.Sprintf("brew install %s", formula))
		} else {
			hints = append(hints, fmt.Sprintf("brew install %s  (install Homebrew first: https://brew.sh)", formula))
		}
	}

	if port, ok := macOSPortPackages[cmdName]; ok && commandExists("port") {
		hints = append(hints, fmt.Sprintf("sudo port install %s", port))
	}

	return hints
}

// ---- dependency list ----

func buildDependencyList() []Dependency {
	return []Dependency{
		{
			Name:         "smartctl",
			Required:     false,
			Feature:      "smart",
			Description:  "SMART disk health monitoring",
			InstallHints: makeDarwinHints("smartctl"),
		},
		{
			// df is a standard macOS built-in; mark it required but no hints needed
			Name:         "df",
			Required:     true,
			Feature:      "disks",
			Description:  "Disk usage statistics (built-in)",
			InstallHints: nil,
		},
		{
			// diskutil is always present on macOS; listing it makes the
			// feature availability visible to the frontend.
			Name:         "diskutil",
			Required:     true,
			Feature:      "disks",
			Description:  "Disk / partition enumeration (built-in)",
			InstallHints: nil,
		},
	}
}
