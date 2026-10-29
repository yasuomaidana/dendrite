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

// runner runs a command and returns its output; an error means the command is not available.
type runner func(name string, args ...string) (string, error)

func execRunner(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).Output()
	return string(out), err
}

// SampleMachine reads swap use, free memory and thermal throttling on macOS; other systems report zeros.
func SampleMachine() Machine { return sampleMachine(execRunner) }

func sampleMachine(run runner) Machine {
	var m Machine
	if out, err := run("sysctl", "-n", "vm.swapusage"); err == nil {
		if g := swapUsed.FindStringSubmatch(out); g != nil {
			m.SwapUsedMB, _ = strconv.ParseFloat(g[1], 64)
		}
	}
	if out, err := run("memory_pressure"); err == nil {
		if g := freePct.FindStringSubmatch(out); g != nil {
			m.FreePct, _ = strconv.Atoi(g[1])
		}
	}
	if out, err := run("pmset", "-g", "therm"); err == nil {
		m.Throttled = strings.Contains(out, "CPU_Speed_Limit") && !strings.Contains(out, "CPU_Speed_Limit = 100")
	}
	return m
}
