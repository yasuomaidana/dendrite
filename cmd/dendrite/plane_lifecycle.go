package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"Dendrite/pkg/planesvc"
	"Dendrite/pkg/ticketlock"
	"Dendrite/pkg/tracker/plane"
)

// lifecycleIn builds the ticket lifecycle for the working copy in worktree ("" is the current directory).
func lifecycleIn(c *plane.Client, worktree string) *ticketlock.Lifecycle {
	if worktree == "" {
		worktree = "."
	}
	if abs, err := filepath.Abs(worktree); err == nil {
		worktree = abs
	}
	return ticketlock.NewLifecycle(ticketlock.Board{Svc: planesvc.Default(), Client: c}, worktree)
}

// readTextArg returns text, or the content of file when text is empty ("-" reads standard input).
func readTextArg(text, file string, stdin io.Reader) (string, error) {
	if text != "" || file == "" {
		return text, nil
	}
	var data []byte
	var err error
	if file == "-" {
		data, err = io.ReadAll(stdin)
	} else {
		data, err = os.ReadFile(file)
	}
	if err != nil {
		return "", fmt.Errorf("read %s: %w", file, err)
	}
	return string(data), nil
}

func newChronosClaimCmd() *cobra.Command {
	var o ticketlock.ClaimOptions
	var worktree string
	cmd := &cobra.Command{
		Use:   "claim <ticket>",
		Short: "Claim a ticket for this session: In Progress, assigned, labelled, and the edit lock opened for this working copy",
		Long: "Claims the ticket for the branch checked out in --worktree (default: the current directory). The ticket must be in\n" +
			"a state that allows work (a Blocked one is resumed) and not held by another live session (--steal takes it over).\n" +
			"It moves the ticket to In Progress, assigns it to the API key's user, labels it agent:<name> and tier:<tier>,\n" +
			"posts a claim comment and records the claim that `dendrite hook pre-edit` checks. The ticket's plan, if any, is shown.",
		Example: "dendrite chronos claim DEN-377\ndendrite chronos claim DEN-377 --worktree ../dendrite-work/Dendrite-DEN-377 --agent claude --tier 8",
		Args:    exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return planeRun(func(c *plane.Client) error {
				return runChronosClaim(cmd.Context(), lifecycleIn(c, worktree), args[0], o, cmd.OutOrStdout())
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&o.Agent, "agent", "", "claude, agy, copilot, ... (default: detected)")
	f.StringVar(&o.Tier, "tier", "", "Model tier of this session (default: DENDRITE_MODEL_TIER)")
	f.BoolVar(&o.Steal, "steal", false, "Take the ticket over from another live session")
	f.StringVar(&worktree, "worktree", "", "Working copy to claim for (default: the current directory)")
	return cmd
}

func runChronosClaim(ctx context.Context, l *ticketlock.Lifecycle, ticket string, o ticketlock.ClaimOptions, out io.Writer) error {
	res, err := l.Claim(ctx, ticket, o)
	if err != nil {
		return err
	}
	if globals.JSON {
		return writeJSON(out, res)
	}
	fmt.Fprintf(out, "[✓] Claimed %s: %s on branch %s (session %s)\n", res.Ticket, res.State, res.Branch, res.SessionID)
	if res.Plan != nil {
		fmt.Fprintf(out, "    Plan v%d from %s: %d step(s) left, next: %s\n", res.Plan.Version, res.Plan.Source, res.Plan.PendingSteps, firstOr(res.Plan.NextStep, "(none)"))
	}
	return nil
}

func firstOr(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

func newChronosCommentCmd() *cobra.Command {
	var kind, body, bodyFile, worktree string
	cmd := &cobra.Command{
		Use:     "comment <ticket>",
		Short:   "Post a structured progress, milestone, note or handoff comment on a ticket",
		Example: "dendrite chronos comment DEN-377 --kind milestone --body \"lock done, 100% coverage\"",
		Args:    exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			text, err := readTextArg(body, bodyFile, cmd.InOrStdin())
			if err != nil {
				return err
			}
			return planeRun(func(c *plane.Client) error {
				if err := lifecycleIn(c, worktree).Comment(cmd.Context(), args[0], kind, text); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "[✓] Posted a %s comment on %s\n", kind, args[0])
				return nil
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&kind, "kind", "progress", "progress, milestone, note or handoff")
	f.StringVar(&body, "body", "", "The comment text")
	f.StringVar(&bodyFile, "body-file", "", "Read the text from a file (- for standard input)")
	f.StringVar(&worktree, "worktree", "", "Working copy the comment is about (default: the current directory)")
	return cmd
}

func newChronosPlanCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "plan", Short: "Store and read the plan of a ticket (kept in Plane and in the local plan store)", Args: noArgs}
	var file, worktree string
	push := &cobra.Command{
		Use:     "push <ticket>",
		Short:   "Store the plan of a ticket: markdown with `- [ ]` / `- [x]` steps (--file, - for standard input)",
		Example: "dendrite chronos plan push DEN-377 --file ~/.claude/plans/my-plan.md",
		Args:    exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if file == "" {
				return usageErrorf("--file is required")
			}
			md, err := readTextArg("", file, cmd.InOrStdin())
			if err != nil {
				return err
			}
			return planeRun(func(c *plane.Client) error {
				p, err := lifecycleIn(c, worktree).UploadPlan(cmd.Context(), args[0], md)
				if err != nil {
					return err
				}
				if globals.JSON {
					return writeJSON(cmd.OutOrStdout(), p)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "[✓] Stored plan v%d of %s: %d step(s) left, next: %s\n", p.Version, args[0], p.PendingSteps, firstOr(p.NextStep, "(none)"))
				return nil
			})
		},
	}
	push.Flags().StringVar(&file, "file", "", "File with the plan (- for standard input)")
	push.Flags().StringVar(&worktree, "worktree", "", "Working copy (default: the current directory)")

	get := &cobra.Command{
		Use:   "get <ticket>",
		Short: "Print the plan of a ticket, from Plane or the local plan store",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return planeRun(func(c *plane.Client) error {
				p, err := lifecycleIn(c, "").GetPlan(cmd.Context(), args[0])
				if err != nil {
					return err
				}
				if globals.JSON {
					return writeJSON(cmd.OutOrStdout(), p)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s\n\n[i] v%d from %s, %d step(s) left, next: %s\n", strings.TrimRight(p.Markdown, "\n"), p.Version, p.Source, p.PendingSteps, firstOr(p.NextStep, "(none)"))
				return nil
			})
		},
	}
	cmd.AddCommand(push, get)
	return cmd
}

func newChronosHandoffCmd() *cobra.Command {
	var h ticketlock.Handoff
	var worktree string
	cmd := &cobra.Command{
		Use:   "handoff <ticket>",
		Short: "Block a ticket with the account the next session needs: what is done, what got in the way, what is missing",
		Long: "Posts the handoff as a comment and moves the ticket to Blocked. All three sections are required: a ticket that\n" +
			"stops without saying where it stopped cannot be resumed. The claim and the plan are kept.",
		Example: "dendrite chronos handoff DEN-377 --completed \"lock + tools\" --challenges \"Plane drops HTML comments\" --missing \"AC-3: round trip\"",
		Args:    exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return planeRun(func(c *plane.Client) error {
				res, err := lifecycleIn(c, worktree).Transition(cmd.Context(), args[0], "blocked", h)
				if err != nil {
					return err
				}
				if globals.JSON {
					return writeJSON(cmd.OutOrStdout(), res)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "[✓] %s is now %s, with its handoff\n", res.Ticket, res.State)
				return nil
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&h.Completed, "completed", "", "What is done")
	f.StringVar(&h.Challenges, "challenges", "", "What got in the way")
	f.StringVar(&h.Missing, "missing", "", "What is missing against the acceptance criteria")
	f.StringVar(&worktree, "worktree", "", "Working copy (default: the current directory)")
	return cmd
}

func newChronosContextCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "context <ticket>",
		Short: "Everything needed to start on a ticket: description, acceptance criteria, plan and the files it names",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return planeRun(func(c *plane.Client) error {
				got, err := lifecycleIn(c, "").QueryContext(cmd.Context(), args[0])
				if err != nil {
					return err
				}
				out := cmd.OutOrStdout()
				if globals.JSON {
					return writeJSON(out, got)
				}
				fmt.Fprintf(out, "%s [%s]\n\n%s\n", got.Name, got.State, got.Description)
				for _, ac := range got.AcceptanceCriteria {
					fmt.Fprintf(out, "  - %s\n", ac)
				}
				if got.Plan != nil {
					fmt.Fprintf(out, "\nPlan v%d (%d step(s) left), next: %s\n", got.Plan.Version, got.Plan.PendingSteps, firstOr(got.Plan.NextStep, "(none)"))
				}
				if len(got.Files) > 0 {
					fmt.Fprintf(out, "\nFiles named: %s\n", strings.Join(got.Files, ", "))
				}
				return nil
			})
		},
	}
}
