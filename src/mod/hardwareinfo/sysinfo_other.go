//go:build !linux && !darwin
// +build !linux,!darwin

package hardwareinfo

import (
	"net/http"
	"os/exec"
	"strings"

	"imuslab.com/bokofs/bokofsd/mod/utils"
)

func sysIfconfig(w http.ResponseWriter, r *http.Request) {
	utils.SendErrorResponse(w, "network interface listing is not supported on this platform")
}

func sysGetDriveStat(w http.ResponseWriter, r *http.Request) {
	utils.SendErrorResponse(w, "drive stat is not supported on this platform")
}

func sysGetUSB(w http.ResponseWriter, r *http.Request) {
	utils.SendErrorResponse(w, "USB listing is not supported on this platform")
}

func sysGetCPUInfo(w http.ResponseWriter, r *http.Request) {
	utils.SendErrorResponse(w, "CPU info is not supported on this platform")
}

func sysGetRamInfo(w http.ResponseWriter, r *http.Request) {
	utils.SendErrorResponse(w, "RAM info is not supported on this platform")
}

// wmicGetinfo queries Windows WMI via the wmic command.
// Only meaningful on Windows; on other platforms the command will not be found.
func wmicGetinfo(wmicName string, itemName string) []string {
	var cmd *exec.Cmd
	if wmicName == "os" {
		cmd = exec.Command("wmic", wmicName, "get", "*", "/format:list")
	} else if len(wmicName) > 6 && wmicName[:6] == "Win32_" {
		cmd = exec.Command("wmic", "path", wmicName, "get", "*", "/format:list")
	} else {
		cmd = exec.Command("wmic", wmicName, "list", "full", "/format:list")
	}

	out, _ := cmd.CombinedOutput()
	var result []string
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			if strings.TrimSpace(parts[0]) == itemName {
				result = append(result, strings.TrimRight(parts[1], "\r"))
			}
		}
	}
	if len(result) == 0 {
		result = append(result, "Undefined")
	}
	return result
}
