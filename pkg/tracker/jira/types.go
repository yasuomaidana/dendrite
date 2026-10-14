package jira

import (
	"errors"
)

// Standard typed errors for Jira provider operations.
var (
	ErrEmptyBaseURL       = errors.New("jira base URL cannot be empty")
	ErrEmptyEmail         = errors.New("jira account email cannot be empty")
	ErrMissingAPIToken    = errors.New("jira API token environment variable is empty or not set")
	ErrUnauthorized       = errors.New("jira request unauthorized: invalid email or API token")
	ErrIssueNotFound      = errors.New("jira issue not found")
	ErrTransitionNotFound = errors.New("no matching jira workflow transition found for requested status")
	ErrCredentialsTimeout = errors.New("jira credential validation timed out")
)

// Config models Jira provider settings.
// Tokens must strictly be resolved from environment variables to prevent plaintext leakage.
type Config struct {
	BaseURL     string `yaml:"base_url" json:"base_url"`
	Email       string `yaml:"email" json:"email"`
	APITokenEnv string `yaml:"api_token_env" json:"api_token_env"`
	ProjectKey  string `yaml:"project_key" json:"project_key"`
}
