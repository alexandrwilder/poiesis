package main

import (
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// machineStruggles is true when the computer is under pressure: little free memory or a
// high load. Poiesis then takes the lighter video by itself. Checked at most once a minute.

var (
	pressureMu   sync.Mutex
	pressureAt   time.Time
	pressureBusy bool
)

func machineStruggles() bool {
	pressureMu.Lock()
	defer pressureMu.Unlock()
	if time.Since(pressureAt) < time.Minute {
		return pressureBusy
	}
	pressureAt = time.Now()
	pressureBusy = false
	if runtime.GOOS != "darwin" {
		return false
	}
	// load average against the cores
	if out, err := exec.Command("sysctl", "-n", "vm.loadavg").Output(); err == nil {
		f := strings.Fields(strings.Trim(strings.TrimSpace(string(out)), "{} "))
		if len(f) > 0 {
			if load, err := strconv.ParseFloat(f[0], 64); err == nil && load > float64(runtime.NumCPU())*0.9 {
				pressureBusy = true
			}
		}
	}
	// on battery, a laptop should not spend a core on a picture
	if out, err := exec.Command("pmset", "-g", "batt").Output(); err == nil && strings.Contains(string(out), "Battery Power") {
		pressureBusy = true
	}
	// free memory below a few hundred megabytes
	if out, err := exec.Command("vm_stat").Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(line, "Pages free:") {
				n, _ := strconv.Atoi(strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "Pages free:")), "."))
				if n*16384 < 300<<20 {
					pressureBusy = true
				}
			}
		}
	}
	return pressureBusy
}
