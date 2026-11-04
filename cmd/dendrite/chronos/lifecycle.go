package chronos

import (
	"Dendrite/cmd/dendrite/internal/cli"
	"Dendrite/cmd/dendrite/internal/planecli"
	"Dendrite/pkg/directive"
	"Dendrite/pkg/ticketlock"
	"Dendrite/pkg/tracker/plane"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

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
		Args:    cli.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return planecli.PlaneRun(func(c *plane.Client) error {
				return runChronosClaim(cmd.Context(), planecli.LifecycleIn(c, worktree), args[0], o, cmd.OutOrStdout())
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
	if cli.Globals.JSON {
		return cli.WriteJSON(out, res)
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
		Args:    cli.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			text, err := readTextArg(body, bodyFile, cmd.InOrStdin())
			if err != nil {
				return err
			}
			return planecli.PlaneRun(func(c *plane.Client) error {
				if err := planecli.LifecycleIn(c, worktree).Comment(cmd.Context(), args[0], kind, text); err != nil {
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
	cmd := &cobra.Command{Use: "plan", Short: "Store and read the plan of a ticket (kept in Plane and in the local plan store)", Args: cli.NoArgs}
	var file, worktree string
	push := &cobra.Command{
		Use:     "push <ticket>",
		Short:   "Store the plan of a ticket: markdown with `- [ ]` / `- [x]` steps (--file, - for standard input)",
		Example: "dendrite chronos plan push DEN-377 --file ~/.claude/plans/my-plan.md",
		Args:    cli.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if file == "" {
				return cli.UsageErrorf("--file is required")
			}
			md, err := readTextArg("", file, cmd.InOrStdin())
			if err != nil {
				return err
			}
			return planecli.PlaneRun(func(c *plane.Client) error {
				p, err := planecli.LifecycleIn(c, worktree).UploadPlan(cmd.Context(), args[0], md)
				if err != nil {
					return err
				}
				if cli.Globals.JSON {
					return cli.WriteJSON(cmd.OutOrStdout(), p)
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
		Args:  cli.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return planecli.PlaneRun(func(c *plane.Client) error {
				p, err := planecli.LifecycleIn(c, "").GetPlan(cmd.Context(), args[0])
				if err != nil {
					return err
				}
				if cli.Globals.JSON {
					return cli.WriteJSON(cmd.OutOrStdout(), p)
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
		Args:    cli.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return planecli.PlaneRun(func(c *plane.Client) error {
				res, err := planecli.LifecycleIn(c, worktree).Transition(cmd.Context(), args[0], "blocked", h)
				if err != nil {
					return err
				}
				if cli.Globals.JSON {
					return cli.WriteJSON(cmd.OutOrStdout(), res)
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
		Args:  cli.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return planecli.PlaneRun(func(c *plane.Client) error {
				got, err := planecli.LifecycleIn(c, "").QueryContext(cmd.Context(), args[0])
				if err != nil {
					return err
				}
				out := cmd.OutOrStdout()
				if cli.Globals.JSON {
					return cli.WriteJSON(out, got)
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

func NewChronosFinishCmd() *cobra.Command {
	var worktree string
	cmd := &cobra.Command{
		Use:   "finish <ticket>",
		Short: "Finish a ticket: quality gate, acceptance review, then Done, To Verify or Blocked with a handoff",
		Long: "Runs the quality gate in the working copy, then has the local model check the branch's diff against every [AC-n]\n" +
			"of the ticket. A failed check blocks the ticket and writes the handoff (completed, challenges, missing). A clean\n" +
			"finish sets Done when the project allows it (`[done] auto_done` in " + directive.Path + ", on by default)\n" +
			"and the ticket has no " + ticketlock.HumanReviewLabel + " label; otherwise, or when the review could not run,\n" +
			"it sets To Verify with the report as a comment. Exits non-zero when the ticket ended Blocked.",
		Example: "dendrite chronos finish DEN-379",
		Args:    cli.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return planecli.PlaneRun(func(c *plane.Client) error {
				l := planecli.LifecycleIn(c, worktree)
				return RunChronosFinish(cmd.Context(), l, args[0], ticketlock.DefaultFinishChecks(l.Dir), cmd.OutOrStdout())
			})
		},
	}
	cmd.Flags().StringVar(&worktree, "worktree", "", "Working copy to finish in (default: the current directory)")
	return cmd
}

func RunChronosFinish(ctx context.Context, l *ticketlock.Lifecycle, ticket string, checks ticketlock.FinishChecks, out io.Writer) error {
	res, err := l.Finish(ctx, ticket, checks)
	if err != nil {
		return err
	}
	if cli.Globals.JSON {
		if err := cli.WriteJSON(out, res); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(out, "[✓] %s is now %s\n", res.Ticket, res.State)
		for _, n := range res.Notes {
			fmt.Fprintf(out, "    - %s\n", n)
		}
	}
	if res.State == "Blocked" {
		return cli.UsageErrorf("%s was blocked: the handoff on the ticket says what failed", res.Ticket)
	}
	return nil
}

func NewChronosDecisionCmd() *cobra.Command {
	var d ticketlock.Decision
	var options []string
	var worktree string
	cmd := &cobra.Command{
		Use:   "decision <ticket>",
		Short: "Record a question and the answer it got (or a decision you took) on a ticket",
		Long: "Posts a decision comment: the question, the answer and, optionally, the options that were on the table.\n" +
			"Claude Code's AskUserQuestion is recorded automatically by `dendrite hook decision`; use this for every\n" +
			"other agent, and for a question asked in plain chat.",
		Example: "dendrite chronos decision DEN-377 --question \"Done rights?\" --answer \"Agents may set Done\" --option \"Human sign-off\" --option \"Agents may set Done\"",
		Args:    cli.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			d.Options = options
			return planecli.PlaneRun(func(c *plane.Client) error {
				if err := planecli.LifecycleIn(c, worktree).RecordDecisions(cmd.Context(), args[0], []ticketlock.Decision{d}, ""); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "[✓] Recorded the decision on %s\n", args[0])
				return nil
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&d.Question, "question", "", "The question that was asked")
	f.StringVar(&d.Answer, "answer", "", "The answer or decision")
	f.StringArrayVar(&options, "option", nil, "An option that was on the table (repeatable)")
	f.StringVar(&d.Note, "note", "", "Why, or what follows from it")
	f.StringVar(&worktree, "worktree", "", "Working copy the decision belongs to (default: the current directory)")
	_ = cmd.MarkFlagRequired("question")
	_ = cmd.MarkFlagRequired("answer")
	return cmd
}

func NewChronosCommentsCmd() *cobra.Command {
	var kind, worktree string
	var last int
	cmd := &cobra.Command{
		Use:     "comments <ticket>",
		Short:   "Read the comments of a ticket: claims, progress, decisions, handoffs and plans",
		Example: "dendrite chronos comments DEN-396 --kind handoff --last 1",
		Args:    cli.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return planecli.PlaneRun(func(c *plane.Client) error {
				rows, err := planecli.LifecycleIn(c, worktree).Comments(cmd.Context(), args[0], kind, last)
				if err != nil {
					return err
				}
				return PrintComments(cmd.OutOrStdout(), rows)
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&kind, "kind", "", "Only comments of this dendrite kind (claim, progress, milestone, note, handoff, decision, plan)")
	f.IntVar(&last, "last", 0, "Only the last N (default: all)")
	f.StringVar(&worktree, "worktree", "", "Working copy to read from (default: the current directory)")
	return cmd
}

func PrintComments(out io.Writer, rows []ticketlock.CommentView) error {
	if cli.Globals.JSON {
		return cli.WriteJSON(out, rows)
	}
	for _, r := range rows {
		fmt.Fprintf(out, "--- %s %s\n%s\n", firstOr(r.Created, "-"), firstOr(r.Kind, "comment"), r.Text)
	}
	if len(rows) == 0 {
		fmt.Fprintln(out, "(no comments)")
	}
	return nil
}
