package linear

import (
	"errors"
)

// Standard typed errors for Linear provider operations.
var (
	ErrMissingAPIToken   = errors.New("linear API key environment variable is empty or not set")
	ErrUnauthorized      = errors.New("linear request unauthorized: invalid API key")
	ErrIssueNotFound     = errors.New("linear issue not found")
	ErrStateNotFound     = errors.New("no matching linear workflow state found for requested status")
	ErrValidationTimeout = errors.New("linear credential validation timed out")
)

// DefaultEndpoint is the standard Linear GraphQL API endpoint.
const DefaultEndpoint = "https://api.linear.app/graphql"

// Config encapsulates settings for the Linear GraphQL provider.
type Config struct {
	Endpoint    string `yaml:"endpoint" json:"endpoint"`
	APITokenEnv string `yaml:"api_token_env" json:"api_token_env"`
	TeamKey     string `yaml:"team_key,omitempty" json:"team_key,omitempty"`
}
