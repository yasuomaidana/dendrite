package inference

import (
	"regexp"
	"strconv"
	"strings"

	"Dendrite/pkg/memfit"
)

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

// Fits reports whether a model, named by its tag, fits the machine's local-model memory budget
// (memfit.BudgetBytes, which the gateway uses too). A tag with no parameter count is given the benefit
// of the doubt.
func Fits(name string) bool {
	return FitsBudget(name, memfit.BudgetBytes())
}

// FitsBudget is Fits against an explicit budget in bytes.
func FitsBudget(name string, budget uint64) bool {
	gb, ok := EstimateModelGB(name)
	if !ok {
		return true
	}
	fits, _ := memfit.Fits(int64(gb*(1<<30)), budget, 0)
	return fits
}
