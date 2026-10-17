package gateway

import "os"

// DefaultCopilotModel is the GitHub Copilot chat model used when DENDRITE_COPILOT_MODEL is unset.
// Copilot retires model IDs over time; list the ones your account offers with
// GET https://api.githubcopilot.com/models.
const DefaultCopilotModel = "claude-sonnet-4.6"

// CopilotModel returns the GitHub Copilot chat model Dendrite routes to.
func CopilotModel() string {
	if m := os.Getenv("DENDRITE_COPILOT_MODEL"); m != "" {
		return m
	}
	return DefaultCopilotModel
}
