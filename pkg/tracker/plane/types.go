package plane

import (
	"errors"
)

// Standard typed errors for Plane v2 provider operations.
var (
	ErrEmptyBaseURL      = errors.New("plane base URL cannot be empty")
	ErrEmptyWorkspace    = errors.New("plane workspace slug cannot be empty")
	ErrEmptyProjectID    = errors.New("plane project ID cannot be empty")
	ErrMissingAPIToken   = errors.New("plane API token environment variable is empty or not set")
	ErrUnauthorized      = errors.New("plane request unauthorized: invalid API key")
	ErrIssueNotFound     = errors.New("plane issue not found")
	ErrStateNotFound     = errors.New("no matching plane state found for requested status")
	ErrValidationTimeout = errors.New("plane credential validation timed out")
)

// Config encapsulates settings for the Plane v2 REST client.
type Config struct {
	BaseURL       string `yaml:"base_url" json:"base_url"`
	WorkspaceSlug string `yaml:"workspace_slug" json:"workspace_slug"`
	ProjectID     string `yaml:"project_id" json:"project_id"`
	APITokenEnv   string `yaml:"api_token_env" json:"api_token_env"`
}
