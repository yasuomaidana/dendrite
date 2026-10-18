package main

import (
	"context"
	"fmt"
	"strings"

	"Dendrite/pkg/personalcontext/greeting"
	"encoding/json"
	"flag"
)

func handleGreetingCommand(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("greeting", flag.ContinueOnError)
	topic := fs.String("topic", "", "User's current topic or project")
	decision := fs.String("decision", "", "Decision point being weighed")
	milestone := fs.String("milestone", "", "Milestone achieved or in-flight")
	milestoneDone := fs.Bool("milestone-done", false, "Whether the milestone was recently completed")
	event := fs.String("event", "", "Upcoming event or deadline")
	daysEvent := fs.Int("days", 0, "Days remaining until event")
	options := fs.String("options", "", "Comma-separated options being weighed")
	unfinished := fs.String("unfinished", "", "Unfinished task from prior turn")
	challenge := fs.String("challenge", "", "Complex or stressful challenge")
	curiosity := fs.String("curiosity", "", "Curiosity angle or serendipitous idea")
	daysInactive := fs.Int("days-inactive", 0, "Days since user last interacted")
	turns := fs.Int("turns", 1, "Number of turns in current conversation")
	stress := fs.Bool("stress", false, "High cognitive load or stress context")
	hobby := fs.Bool("hobby", false, "Low-pressure creative hobby context")
	suppress := fs.String("suppress", "", "Comma-separated style IDs to suppress (e.g. G-01,G-02)")
	asJSON := fs.Bool("json", false, "Output results in JSON format")

	if err := fs.Parse(args); err != nil {
		return
	}

	uCtx := greeting.UserJourneyContext{
		Topic:               *topic,
		Decision:            *decision,
		Milestone:           *milestone,
		MilestoneAchieved:   *milestoneDone || (*milestone != ""),
		Event:               *event,
		DaysUntilEvent:      *daysEvent,
		UnfinishedStep:      *unfinished,
		SpecificChallenge:   *challenge,
		CuriosityAngle:      *curiosity,
		DaysSinceLastActive: *daysInactive,
		TurnCount:           *turns,
		HighStress:          *stress,
		LowPressureHobby:    *hobby,
	}

	if *options != "" {
		for _, opt := range strings.Split(*options, ",") {
			if trimmed := strings.TrimSpace(opt); trimmed != "" {
				uCtx.Options = append(uCtx.Options, trimmed)
			}
		}
	}

	var recentStyles []greeting.StyleID
	if *suppress != "" {
		for _, s := range strings.Split(*suppress, ",") {
			if trimmed := strings.TrimSpace(s); trimmed != "" {
				recentStyles = append(recentStyles, greeting.StyleID(trimmed))
			}
		}
	}

	eng := greeting.NewEngine()
	res, err := eng.Generate(ctx, uCtx, recentStyles)
	if err != nil {
		fmt.Printf("[-] Failed to generate greeting: %v\n", err)
		return
	}

	if *asJSON {
		data, _ := json.MarshalIndent(res, "", "  ")
		fmt.Println(string(data))
		return
	}

	fmt.Println("[+] Dynamic 10-Style Greeting Matrix Resolution:")
	fmt.Printf("    - Style:    [%s] %s\n", res.StyleID, res.StyleName)
	fmt.Printf("    - Headline: %s\n", res.Headline)
	fmt.Printf("    - Template: %s\n", res.TemplateUsed)
}
