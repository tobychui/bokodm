//go:build darwin
// +build darwin

package hardwareinfo

import (
	"encoding/json"
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"strings"

	"imuslab.com/bokodm/bokodmd/mod/utils"
)

func sysIfconfig(w http.ResponseWriter, r *http.Request) {
	out, err := exec.Command("ifconfig").CombinedOutput()
	if err != nil {
		out = []byte{}
	}
	jsonData, err := json.Marshal(strings.Split(string(out), "\n"))
	if err != nil {
		log.Println(err)
	}
	utils.SendTextResponse(w, string(jsonData))
}

func sysGetDriveStat(w http.ResponseWriter, r *http.Request) {
	// -Pk gives a 6-column POSIX layout on macOS, same as Linux
	out, err := exec.Command("bash", "-c", `df -Pk | sed -e /Filesystem/d`).Output()
	if err != nil {
		out = []byte{}
	}

	var arr []LogicalDisk
	for _, line := range strings.Split(string(out), "\n") {
		if line == "" {
			continue
		}
		for strings.Contains(line, "  ") {
			line = strings.Replace(line, "  ", " ", -1)
		}
		chunks := strings.Split(line, " ")
		if len(chunks) < 6 {
			continue
		}
		tmp, _ := strconv.Atoi(chunks[3])
		arr = append(arr, LogicalDisk{
			DriveLetter: chunks[5],
			FileSystem:  chunks[0],
			FreeSpace:   strconv.FormatInt(int64(tmp)*1024, 10),
		})
	}

	jsonData, err := json.Marshal(arr)
	if err != nil {
		log.Println(err)
	}
	utils.SendTextResponse(w, string(jsonData))
}

func sysGetUSB(w http.ResponseWriter, r *http.Request) {
	out, err := exec.Command("system_profiler", "SPUSBDataType").CombinedOutput()
	if err != nil {
		out = []byte{}
	}
	jsonData, err := json.Marshal(strings.Split(string(out), "\n"))
	if err != nil {
		log.Println(err)
	}
	utils.SendTextResponse(w, string(jsonData))
}

func sysGetCPUInfo(w http.ResponseWriter, r *http.Request) {
	model := sysctlString("machdep.cpu.brand_string")
	if model == "" {
		model = sysctlString("hw.model")
	}

	freq := ""
	if hz := sysctlInt64("hw.cpufrequency_max"); hz > 0 {
		freq = strconv.FormatInt(hz/1_000_000, 10) + " MHz"
	} else if tbFreq := sysctlInt64("hw.tbfrequency"); tbFreq > 0 {
		freq = strconv.FormatInt(tbFreq/1_000_000, 10) + " MHz (timebase)"
	}

	arch, _ := exec.Command("uname", "-m").Output()

	info := CPUInfo{
		Model:       strings.TrimSpace(model),
		Freq:        freq,
		Instruction: strings.TrimSpace(string(arch)),
		Hardware:    sysctlString("hw.model"),
		Revision:    "",
	}

	jsonData, err := json.Marshal(info)
	if err != nil {
		log.Println(err)
	}
	utils.SendTextResponse(w, string(jsonData))
}

func sysGetRamInfo(w http.ResponseWriter, r *http.Request) {
	ramBytes := sysctlInt64("hw.memsize")
	jsonData, _ := json.Marshal(ramBytes)
	utils.SendTextResponse(w, string(jsonData))
}

func sysctlString(name string) string {
	out, err := exec.Command("sysctl", "-n", name).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func sysctlInt64(name string) int64 {
	out, err := exec.Command("sysctl", "-n", name).Output()
	if err != nil {
		return 0
	}
	n, _ := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	return n
}
