package bench

import (
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

var (
	swapUsed = regexp.MustCompile(`used = ([0-9.]+)M`)
	freePct  = regexp.MustCompile(`free percentage: (\d+)%`)
)

// SampleMachine reads swap use, free memory and thermal throttling on macOS; other systems report zeros.
func SampleMachine() Machine {
	var m Machine
	if out, err := exec.Command("sysctl", "-n", "vm.swapusage").Output(); err == nil {
		if g := swapUsed.FindStringSubmatch(string(out)); g != nil {
			m.SwapUsedMB, _ = strconv.ParseFloat(g[1], 64)
		}
	}
	if out, err := exec.Command("memory_pressure").Output(); err == nil {
		if g := freePct.FindStringSubmatch(string(out)); g != nil {
			m.FreePct, _ = strconv.Atoi(g[1])
		}
	}
	if out, err := exec.Command("pmset", "-g", "therm").Output(); err == nil {
		m.Throttled = strings.Contains(string(out), "CPU_Speed_Limit") && !strings.Contains(string(out), "CPU_Speed_Limit = 100")
	}
	return m
}
