package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"Dendrite/pkg/devtool"
)

type quantalOptions struct {
	TicketID      string
	Message       string
	Files         []string
	StageAll      bool
	Amend         bool
	NoPush        bool
	Smart         bool
	Model         string
	UseLocalModel bool
	DryRun        bool
	AllowMocks    bool
}

// newQuantalCmd builds `dendrite quantal`: autonomous Git commit with local model message generation.
func newQuantalCmd() *cobra.Command {
	var o quantalOptions
	cmd := &cobra.Command{
		Use:     "quantal",
		Aliases: []string{"commit", "quantal-commit", "cmt"},
		Short:   "Autonomous Git commit with local model message generation",
		GroupID: "boutons",
		Args:    noArgs,
		Example: "dendrite quantal --ticket DEN-1028\ndendrite quantal -m \"feat(cli): migrate commands to cobra\" --no-push\ndendrite quantal --files cmd/dendrite/commit.go --dry-run",
		RunE: func(c *cobra.Command, args []string) error {
			return runQuantal(c.Context(), o, globals.JSON, c.OutOrStdout())
		},
	}

	f := cmd.Flags()
	addTicketFlag(cmd, &o.TicketID, "")
	f.StringVarP(&o.Message, "message", "m", "", "Explicit commit message (skips local model generation)")
	f.StringSliceVarP(&o.Files, "files", "f", nil, "Comma-separated files to stage")
	f.BoolVarP(&o.StageAll, "all", "a", false, "Stage all modified and untracked files")
	f.BoolVar(&o.Amend, "amend", false, "Amend previous commit instead of creating a new commit")
	f.BoolVar(&o.NoPush, "no-push", false, "Disable pushing commit to remote")
	f.BoolVar(&o.Smart, "smart", true, "Enable smart ticket auto-detection")
	addModelFlag(cmd, &o.Model, "qwen3.8:27b")
	f.BoolVar(&o.UseLocalModel, "local", true, "Enable local model delegation for message synthesis")
	addDryRunFlag(cmd, &o.DryRun)
	f.BoolVar(&o.AllowMocks, "allow-mocks", false, "Bypass zero-mock policy gate for staged files")
	cmd.MarkFlagsMutuallyExclusive("all", "files")

	return cmd
}

func runQuantal(ctx context.Context, o quantalOptions, asJSON bool, out io.Writer) error {
	if o.StageAll && len(o.Files) > 0 {
		return usageErrorf("--all and --files are mutually exclusive")
	}

	opts := devtool.CommitOptions{
		TicketID:        o.TicketID,
		Message:         o.Message,
		Files:           o.Files,
		StageAll:        o.StageAll,
		Amend:           o.Amend,
		NoPush:          o.NoPush,
		Smart:           o.Smart,
		NoSmart:         !o.Smart,
		Model:           o.Model,
		UseLocalModel:   o.UseLocalModel,
		DryRun:          o.DryRun,
		AllowMocks:      o.AllowMocks,
		LiteLLMEndpoint: gatewayV1URL(),
	}

	if strings.EqualFold(os.Getenv("DENDRITE_COMMIT_PUSH"), "false") {
		opts.NoPush = true
	}

	if !asJSON {
		fmt.Fprintln(out, "[+] Initializing Autonomous Git Commit Engine...")
		if opts.TicketID != "" {
			fmt.Fprintf(out, "    - Active Ticket:    %s\n", opts.TicketID)
		} else if !opts.NoSmart {
			fmt.Fprintln(out, "    - Smart Strategy:   Auto-resolving active ticket and context from Plane / DAG...")
		}
		if opts.Amend {
			fmt.Fprintln(out, "    - Commit Mode:      AMEND (--amend)")
		}
		if opts.UseLocalModel && opts.Message == "" {
			fmt.Fprintf(out, "    - Message Model:    %s (Delegated Local Engine)\n", opts.Model)
		} else if opts.Message != "" {
			fmt.Fprintln(out, "    - Message Source:   User Specified")
		}
		if opts.DryRun {
			fmt.Fprintln(out, "    - Execution Mode:   DRY-RUN (Preview Only)")
		}
	}

	res, err := devtool.ExecuteCommit(ctx, opts)
	if err != nil {
		return fmt.Errorf("commit failed: %w", err)
	}

	if asJSON {
		if res.PushError != nil {
			return fmt.Errorf("git push failed: %w", res.PushError)
		}
		data, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal commit result: %w", err)
		}
		fmt.Fprintln(out, string(data))
		return nil
	}

	if res.DryRun {
		fmt.Fprintln(out, "[✓] Dry run commit preview generated:")
		if res.SmartTicket != nil && res.SmartTicket.ID != "" {
			fmt.Fprintf(out, "    - Smart Strategy:   Resolved [%s] via %s\n", res.SmartTicket.ID, res.SmartSource)
			if res.SmartTicket.Title != "" {
				fmt.Fprintf(out, "      * Title:          %s\n", res.SmartTicket.Title)
			}
		}
		fmt.Fprintf(out, "    - Proposed Message:\n%s\n", indentText(res.Message, "      "))
		fmt.Fprintf(out, "    - Staged Files (%d):\n", len(res.Files))
		for _, f := range res.Files {
			fmt.Fprintf(out, "      * %s\n", f)
		}
		fmt.Fprintf(out, "    - Model Engine:     %s (Cost: $%.2f)\n", res.ModelUsed, res.CostUSD)
		if res.PushSkipped {
			fmt.Fprintf(out, "    - Remote Push:      Skipped (%s)\n", res.PushReason)
		}
		return nil
	}

	fmt.Fprintf(out, "[✓] Commit applied successfully (%s)\n", res.CommitSHA[:min(8, len(res.CommitSHA))])
	if res.SmartTicket != nil && res.SmartTicket.ID != "" {
		fmt.Fprintf(out, "    - Smart Strategy:   Resolved [%s] via %s\n", res.SmartTicket.ID, res.SmartSource)
		if res.SmartTicket.Title != "" {
			fmt.Fprintf(out, "      * Title:          %s\n", res.SmartTicket.Title)
		}
	}
	fmt.Fprintf(out, "    - Message:\n%s\n", indentText(res.Message, "      "))
	fmt.Fprintf(out, "    - Files Committed (%d):\n", len(res.Files))
	for _, f := range res.Files {
		fmt.Fprintf(out, "      * %s\n", f)
	}
	fmt.Fprintf(out, "    - Model Provenance: %s (Cost: $%.2f)\n", res.ModelUsed, res.CostUSD)
	if res.Pushed {
		fmt.Fprintln(out, "    - Remote Push:      Pushed to remote")
	} else if res.PushSkipped {
		fmt.Fprintf(out, "    - Remote Push:      Skipped (%s)\n", res.PushReason)
	}

	if res.PushError != nil {
		return fmt.Errorf("git push failed: %w", res.PushError)
	}
	return nil
}

func handleCommitCommand(ctx context.Context, args []string) {
	_ = invokeCmd(ctx, newQuantalCmd(), args...)
}

func indentText(text, indent string) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		lines[i] = indent + l
	}
	return strings.Join(lines, "\n")
}
