package main

/*
	dep.go

	Shared types for runtime dependency tracking.
	The populated DependencyReport is stored in the global runtimeDeps variable
	after startup and served at GET /api/info/deps for the frontend to consume.

	Platform-specific dependency lists live in:
	  dep_linux.go  — Linux (apt/dnf/yum/pacman/zypper/apk hints)
	  dep_darwin.go — macOS (brew/port hints)
	  dep_other.go  — stub for other platforms
*/

import "fmt"

// Dependency describes a single external tool that bokodm depends on.
type Dependency struct {
	// Name is the binary name used in PATH look-up (e.g. "smartctl").
	Name string `json:"name"`
	// Found is true when the binary was found on PATH at startup.
	Found bool `json:"found"`
	// Required marks the dependency as critical; if it is missing and
	// -skip_dep is not set, the server refuses to start.
	Required bool `json:"required"`
	// Feature is a short tag used by the frontend to gate UI sections.
	// Known values: "smart", "raid", "disks", "media"
	Feature string `json:"feature"`
	// Description is a human-readable sentence explaining the role of
	// this dependency.
	Description string `json:"description"`
	// InstallHints contains ready-to-run install commands ordered from
	// most to least preferred, e.g. ["brew install smartmontools"].
	InstallHints []string `json:"installHints"`
}

// DependencyReport is the full dependency check result produced at startup.
// It is served as JSON at GET /api/info/deps.
type DependencyReport struct {
	// AllSatisfied is true when every Required dependency was found.
	AllSatisfied bool `json:"allSatisfied"`
	// DegradedMode is true when the server was started with -skip_dep
	// while one or more Required dependencies were missing.
	DegradedMode bool `json:"degradedMode"`
	// Deps is the complete list of checked dependencies.
	Deps []Dependency `json:"deps"`
}

// IsFeatureAvailable reports whether every dependency tagged with feature
// was found on the system.
func (r *DependencyReport) IsFeatureAvailable(feature string) bool {
	if r == nil {
		return false
	}
	for _, d := range r.Deps {
		if d.Feature == feature && !d.Found {
			return false
		}
	}
	return true
}

// printDepCheck writes a coloured status line and, when the binary is
// missing, any available install hints.
func printDepCheck(dep *Dependency) {
	if dep.Found {
		fmt.Printf("\033[32m✔\033[0m %-12s — %s\n", dep.Name, dep.Description)
		return
	}

	// Yellow ⚠ for optional, red ✘ for required
	symbol := "\033[33m⚠\033[0m"
	if dep.Required {
		symbol = "\033[31m✘\033[0m"
	}
	fmt.Printf("%s %-12s — %s  \033[2m(not found)\033[0m\n", symbol, dep.Name, dep.Description)
	for _, hint := range dep.InstallHints {
		fmt.Printf("    \033[36m💡\033[0m  %s\n", hint)
	}
}
