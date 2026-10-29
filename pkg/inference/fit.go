package inference

import (
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// DefaultFitFraction is the share of RAM a single resident chat model may take. Beyond it the
// system, the embedder and the editor are pushed into swap and every token gets slower, which is
// worse than a smaller model; on a 24 GB machine it admits a 14 GB model and refuses an 18 GB one.
const DefaultFitFraction = 0.65

// FitFractionEnv overrides DefaultFitFraction (for example 0.8 after raising a GPU memory limit).
const FitFractionEnv = "DENDRITE_MODEL_FIT_FRACTION"

var paramCount = regexp.MustCompile(`(?i)(?:^|[^0-9a-z.])(\d+(?:\.\d+)?)b(?:[^a-z]|$)`)

// EstimateModelGB estimates the resident size of a model from its tag (for example "gpt-oss:20b" or
// "qwen3-coder:30b-a3b"): total parameters at about 0.6 GB per billion, which is 4-bit weights plus
// runtime overhead. A mixture-of-experts model keeps all its experts resident, so total parameters
// are what count. ok is false when the tag carries no parameter count.
func EstimateModelGB(name string) (gb float64, ok bool) {
	m := paramCount.FindStringSubmatch(strings.ToLower(name))
	if m == nil {
		return 0, false
	}
	b, err := strconv.ParseFloat(m[1], 64)
	if err != nil || b <= 0 {
		return 0, false
	}
	return b * 0.6, true
}

// Fits reports whether the model can be the machine's resident chat model. A model whose size cannot
// be estimated is given the benefit of the doubt, and so is an unknown amount of RAM.
func Fits(name string, ramGB, fraction float64) bool {
	if ramGB <= 0 {
		return true
	}
	if fraction <= 0 {
		fraction = DefaultFitFraction
	}
	gb, ok := EstimateModelGB(name)
	return !ok || gb <= ramGB*fraction
}

// FitFraction is DefaultFitFraction unless FitFractionEnv says otherwise.
func FitFraction() float64 {
	if v, err := strconv.ParseFloat(os.Getenv(FitFractionEnv), 64); err == nil && v > 0 && v <= 1 {
		return v
	}
	return DefaultFitFraction
}

// MachineRAMGB is the machine's physical memory in GB, or 0 when it cannot be read.
func MachineRAMGB() float64 {
	switch runtime.GOOS {
	case "darwin":
		if out, err := exec.Command("sysctl", "-n", "hw.memsize").Output(); err == nil {
			if b, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64); err == nil {
				return b / (1 << 30)
			}
		}
	case "linux":
		if data, err := os.ReadFile("/proc/meminfo"); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if f := strings.Fields(line); len(f) >= 2 && f[0] == "MemTotal:" {
					if kb, err := strconv.ParseFloat(f[1], 64); err == nil {
						return kb / (1 << 20)
					}
				}
			}
		}
	}
	return 0
}
