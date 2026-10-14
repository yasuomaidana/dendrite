package tokenopt

// CalculateEfficiency returns the ratio of accepted code lines to prompt tokens consumed.
func CalculateEfficiency(acceptedLOC, promptTokensConsumed int) *TokenEfficiencyMetrics {
	var ratio float64
	if promptTokensConsumed > 0 {
		ratio = float64(acceptedLOC) / float64(promptTokensConsumed)
	}

	return &TokenEfficiencyMetrics{
		AcceptedLOC:          acceptedLOC,
		PromptTokensConsumed: promptTokensConsumed,
		EfficiencyRatio:      ratio,
	}
}
