package main

import (
	"fmt"
	"strings"

	"Dendrite/pkg/personalcontext/greeting"

	"github.com/spf13/cobra"
)

// newGreetingCmd builds `dendrite greeting`: a context-aware greeting that avoids recently used styles.
func newGreetingCmd() *cobra.Command {
	var uCtx greeting.UserJourneyContext
	var options, suppress []string
	cmd := &cobra.Command{
		Use:     "greeting",
		Aliases: []string{"greet", "pcontext-greeting"},
		Short:   "Generate dynamic 10-style greeting with cliché suppression",
		GroupID: "cleft",
		Args:    noArgs,
		Example: "dendrite greeting --topic DEN-985\n" +
			"dendrite greeting --event launch --days 3 --stress\n" +
			"dendrite greeting --suppress G-01,G-02 --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			uCtx.MilestoneAchieved = uCtx.MilestoneAchieved || uCtx.Milestone != ""
			uCtx.Options = options
			styles := make([]greeting.StyleID, 0, len(suppress))
			for _, s := range suppress {
				if s = strings.TrimSpace(s); s != "" {
					styles = append(styles, greeting.StyleID(s))
				}
			}
			res, err := greeting.NewEngine().Generate(cmd.Context(), uCtx, styles)
			if err != nil {
				return fmt.Errorf("generate greeting: %w", err)
			}
			out := cmd.OutOrStdout()
			if globals.JSON {
				return writeJSON(out, res)
			}
			fmt.Fprintln(out, "[+] Dynamic 10-Style Greeting Matrix Resolution:")
			fmt.Fprintf(out, "    - Style:    [%s] %s\n", res.StyleID, res.StyleName)
			fmt.Fprintf(out, "    - Headline: %s\n", res.Headline)
			fmt.Fprintf(out, "    - Template: %s\n", res.TemplateUsed)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&uCtx.Topic, "topic", "", "Current topic or project")
	f.StringVar(&uCtx.Decision, "decision", "", "Decision being weighed")
	f.StringSliceVar(&options, "options", nil, "Options being weighed (comma-separated)")
	f.StringVar(&uCtx.Milestone, "milestone", "", "Milestone reached or in flight")
	f.BoolVar(&uCtx.MilestoneAchieved, "milestone-done", false, "The milestone was just completed")
	f.StringVar(&uCtx.Event, "event", "", "Upcoming event or deadline")
	f.IntVar(&uCtx.DaysUntilEvent, "days", 0, "Days until the event")
	f.StringVar(&uCtx.UnfinishedStep, "unfinished", "", "Unfinished task from the last turn")
	f.StringVar(&uCtx.SpecificChallenge, "challenge", "", "A hard or stressful challenge")
	f.StringVar(&uCtx.CuriosityAngle, "curiosity", "", "A curiosity angle or idea")
	f.IntVar(&uCtx.DaysSinceLastActive, "days-inactive", 0, "Days since the last interaction")
	f.IntVar(&uCtx.TurnCount, "turns", 1, "Turns in the current conversation")
	f.BoolVar(&uCtx.HighStress, "stress", false, "High cognitive load or stress")
	f.BoolVar(&uCtx.LowPressureHobby, "hobby", false, "Low-pressure hobby context")
	f.StringSliceVar(&suppress, "suppress", nil, "Style IDs to avoid (e.g. G-01,G-02)")
	return cmd
}
