//go:build linux
// +build linux

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
	cmd := exec.Command("bash", "-c", "ip link show")
	out, err := cmd.CombinedOutput()
	if err != nil {
		out = []byte{}
	}
	lines := strings.Split(string(out), "\n")
	jsonData, err := json.Marshal(lines)
	if err != nil {
		log.Println(err)
	}
	utils.SendTextResponse(w, string(jsonData))
}

func sysGetDriveStat(w http.ResponseWriter, r *http.Request) {
	cmd := exec.Command("bash", "-c", `df -Pk | sed -e /Filesystem/d`)
	out, err := cmd.Output()
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
	out, err := exec.Command("lsusb").CombinedOutput()
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
	model, _ := exec.Command("bash", "-c", `lscpu | grep -m1 "Model name"`).CombinedOutput()
	freq, err := exec.Command("bash", "-c", `lscpu | grep "CPU max MHz"`).CombinedOutput()
	if err != nil {
		freq, _ = exec.Command("bash", "-c", `cat /proc/cpuinfo | grep -m1 "cpu MHz"`).CombinedOutput()
	}
	hardware, _ := exec.Command("bash", "-c", `cat /proc/cpuinfo | grep -m1 "Hardware"`).CombinedOutput()
	revision, err := exec.Command("bash", "-c", `cat /proc/cpuinfo | grep -m1 "Revision"`).CombinedOutput()
	if err != nil {
		revision, _ = exec.Command("bash", "-c", `cat /proc/cpuinfo | grep -m1 "family"`).CombinedOutput()
	}
	arch, _ := exec.Command("uname", "-m").CombinedOutput()

	info := CPUInfo{
		Model:       filterGrepResult(string(model), ":"),
		Freq:        filterGrepResult(string(freq), ":"),
		Hardware:    filterGrepResult(string(hardware), ":"),
		Instruction: filterGrepResult(string(arch), ":"),
		Revision:    filterGrepResult(string(revision), ":"),
	}

	jsonData, err := json.Marshal(info)
	if err != nil {
		log.Println(err)
	}
	utils.SendTextResponse(w, string(jsonData))
}

func sysGetRamInfo(w http.ResponseWriter, r *http.Request) {
	out, _ := exec.Command("grep", "MemTotal", "/proc/meminfo").CombinedOutput()
	s := strings.TrimSpace(string(out))
	s = strings.ReplaceAll(s, "MemTotal:", "")
	s = strings.ReplaceAll(s, "kB", "")
	s = strings.TrimSpace(s)
	ramKB, _ := strconv.ParseInt(s, 10, 64)

	jsonData, _ := json.Marshal(ramKB * 1000)
	utils.SendTextResponse(w, string(jsonData))
}

// filterGrepResult trims the key portion of a "Key: Value" grep result.
func filterGrepResult(result, sep string) string {
	if !strings.Contains(result, sep) {
		return strings.TrimSpace(result)
	}
	parts := strings.SplitN(result, sep, 2)
	return strings.TrimSpace(parts[1])
}
