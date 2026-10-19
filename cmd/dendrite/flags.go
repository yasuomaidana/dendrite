package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// globalOptions holds the persistent flags shared by every command.
type globalOptions struct {
	JSON       bool
	Yes        bool
	StateDir   string
	Home       string
	GatewayURL string
	PlaneURL   string
}

// globals is populated by the root command's persistent flags before any RunE executes.
var globals globalOptions

const defaultGatewayURL = "http://localhost:4000"

// bindGlobalFlags registers the persistent flags on the root command.
func bindGlobalFlags(root *cobra.Command) {
	globals = globalOptions{}
	pf := root.PersistentFlags()
	pf.BoolVar(&globals.JSON, "json", false, "Emit machine-readable JSON output")
	pf.BoolVarP(&globals.Yes, "yes", "y", false, "Assume yes for every interactive prompt")
	pf.StringVar(&globals.StateDir, "state-dir", "", "Dendrite state directory (env DENDRITE_STATE_DIR)")
	pf.StringVar(&globals.Home, "home", "", "Dendrite home holding the global project index (env DENDRITE_HOME)")
	pf.StringVar(&globals.GatewayURL, "gateway-url", "", "LiteLLM gateway base URL (env LITELLM_PROXY_URL, default "+defaultGatewayURL+")")
	pf.StringVar(&globals.PlaneURL, "plane-url", "", "Plane API base URL (env PLANE_BASE_URL)")
}

// applyGlobals exports the global flags to the environment variables read by pkg/ packages.
func applyGlobals() error {
	pairs := []struct{ value, env string }{
		{globals.StateDir, "DENDRITE_STATE_DIR"},
		{globals.Home, "DENDRITE_HOME"},
		{globals.GatewayURL, "LITELLM_PROXY_URL"},
		{globals.PlaneURL, "PLANE_BASE_URL"},
	}
	for _, p := range pairs {
		if p.value == "" {
			continue
		}
		if err := os.Setenv(p.env, p.value); err != nil {
			return fmt.Errorf("set %s: %w", p.env, err)
		}
	}
	return nil
}

// gatewayBaseURL returns the LiteLLM gateway URL without a trailing /v1.
func gatewayBaseURL() string {
	u := globals.GatewayURL
	if u == "" {
		u = os.Getenv("LITELLM_PROXY_URL")
	}
	if u == "" {
		u = defaultGatewayURL
	}
	return strings.TrimSuffix(strings.TrimRight(u, "/"), "/v1")
}

// gatewayV1URL returns the OpenAI-compatible /v1 endpoint of the LiteLLM gateway.
func gatewayV1URL() string {
	return gatewayBaseURL() + "/v1"
}

// usageError marks invocation mistakes (unknown subcommand, bad flags or arguments); it exits with ExitUsage.
type usageError struct{ err error }

func (e *usageError) Error() string { return e.err.Error() }
func (e *usageError) Unwrap() error { return e.err }

// usageErrorf builds a usageError.
func usageErrorf(format string, a ...any) error {
	return &usageError{err: fmt.Errorf(format, a...)}
}

// isUsageError reports whether err is an invocation mistake rather than a runtime failure.
func isUsageError(err error) bool {
	var ue *usageError
	if errors.As(err, &ue) {
		return true
	}
	// cobra reports unknown root commands and flag-group violations with plain errors.
	msg := err.Error()
	for _, p := range []string{"unknown command ", "required flag(s) ", "if any flags in the group ", "at least one of the flags in the group "} {
		if strings.HasPrefix(msg, p) {
			return true
		}
	}
	return false
}

// asUsage wraps a cobra.PositionalArgs validator so its failures exit with ExitUsage.
func asUsage(v cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := v(cmd, args); err != nil {
			return &usageError{err: err}
		}
		return nil
	}
}

// exactArgs, minArgs, maxArgs, rangeArgs and noArgs are the usage-aware positional validators.
func exactArgs(n int) cobra.PositionalArgs      { return asUsage(cobra.ExactArgs(n)) }
func minArgs(n int) cobra.PositionalArgs        { return asUsage(cobra.MinimumNArgs(n)) }
func maxArgs(n int) cobra.PositionalArgs        { return asUsage(cobra.MaximumNArgs(n)) }
func rangeArgs(lo, hi int) cobra.PositionalArgs { return asUsage(cobra.RangeArgs(lo, hi)) }

var noArgs = asUsage(cobra.NoArgs)

// requireFlags marks flags as required; cobra reports missing ones through the flag error func.
func requireFlags(cmd *cobra.Command, names ...string) {
	for _, n := range names {
		if err := cmd.MarkFlagRequired(n); err != nil {
			panic(fmt.Sprintf("requireFlags %s: %v", n, err))
		}
	}
}

// addDryRunFlag registers the shared --dry-run flag.
func addDryRunFlag(cmd *cobra.Command, p *bool) {
	cmd.Flags().BoolVar(p, "dry-run", false, "Show what would happen without changing anything")
}

// addTicketFlag registers the shared --ticket/-t flag.
func addTicketFlag(cmd *cobra.Command, p *string, def string) {
	cmd.Flags().StringVarP(p, "ticket", "t", def, "Ticket identifier (e.g. DEN-123)")
}

// addModelFlag registers the shared --model flag.
func addModelFlag(cmd *cobra.Command, p *string, def string) {
	cmd.Flags().StringVar(p, "model", def, "LLM model name")
}

// addProjectFlag registers the shared --project flag.
func addProjectFlag(cmd *cobra.Command, p *string, def, usage string) {
	cmd.Flags().StringVar(p, "project", def, usage)
}

// addFileFlag registers the shared --file/-f flag.
func addFileFlag(cmd *cobra.Command, p *string, def, usage string) {
	cmd.Flags().StringVarP(p, "file", "f", def, usage)
}

// writeJSON prints v as indented JSON; commands call it when globals.JSON is set.
func writeJSON(out io.Writer, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("encode JSON: %w", err)
	}
	_, err = fmt.Fprintln(out, string(data))
	return err
}
