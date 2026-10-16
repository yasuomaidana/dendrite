package greeting

import (
	"context"
	"strings"
	"testing"
)

func TestValidateHeadline_Valid(t *testing.T) {
	validHeadlines := []string{
		"# How's your trip planning coming along?",
		"# That's a big one — milestone reached!",
		"# A lot has happened with your project — worth a quick look back?",
		"# You were weighing Option A vs Option B — any verdict?",
		"# Draft budget is right here whenever you're ready.",
		"# Only 5 days until conference — exciting stuff!",
		"# Been a little while — landscape photography notes are still here.",
		"# Here's something you might not have considered for your garden...",
		"# Navigating permit delays takes patience — and you've got it.",
		"# Nothing urgent — just your novel outlines, whenever.",
	}

	for _, hl := range validHeadlines {
		if err := ValidateHeadline(hl); err != nil {
			t.Errorf("expected valid headline for %q, got error: %v", hl, err)
		}
	}
}

func TestValidateHeadline_InvalidAndBannedCliches(t *testing.T) {
	invalidCases := []struct {
		headline string
		wantErr  error
	}{
		{"No hash prefix greeting", ErrInvalidGreetingHeadline},
		{"# ", ErrInvalidGreetingHeadline},
		{"# Ready to dive into your work today?", ErrBannedCliche},
		{"# Let's get started on your finances.", ErrBannedCliche},
		{"# Lets wrap this project up.", ErrBannedCliche},
		{"# Time to tackle the backyard project.", ErrBannedCliche},
		{"# Looking to find a new car?", ErrBannedCliche},
		{"# Want to review the latest notes?", ErrBannedCliche},
		{"# How about we check the itinerary?", ErrBannedCliche},
		{"# I hope this finds you well today.", ErrBannedCliche},
		{"# As an AI language model I am here to help.", ErrBannedCliche},
		{"# Certainly here is your morning update.", ErrBannedCliche},
		{"# Great question about your trip.", ErrBannedCliche},
		{"# Unresolved [topic] token left behind.", ErrUnresolvedTemplateToken},
		{"# Sentence one. Sentence two.", ErrInvalidGreetingHeadline},
		{"# " + strings.Repeat("A very long headline that goes on and on and on and on and on and on ", 3), ErrHeadlineTooLong},
	}

	for _, c := range invalidCases {
		err := ValidateHeadline(c.headline)
		if err == nil {
			t.Errorf("expected error for %q, got nil", c.headline)
			continue
		}
		if c.wantErr != nil && !strings.Contains(err.Error(), c.wantErr.Error()) {
			t.Errorf("for %q, expected error containing %v, got %v", c.headline, c.wantErr, err)
		}
	}
}

func TestEngine_All10StylesGeneration(t *testing.T) {
	eng := NewEngine()
	ctx := context.Background()

	testScenarios := []struct {
		name          string
		uCtx          UserJourneyContext
		expectedStyle StyleID
	}{
		{
			name: "G-02 Progress celebration",
			uCtx: UserJourneyContext{
				Topic:             "Home Renovation",
				Milestone:         "Kitchen demo",
				MilestoneAchieved: true,
			},
			expectedStyle: StyleProgressCelebration,
		},
		{
			name: "G-04 Callback to specific detail",
			uCtx: UserJourneyContext{
				Topic:   "Laptop Purchase",
				Options: []string{"MacBook Pro 16", "ThinkPad P1"},
			},
			expectedStyle: StyleCallbackToDetail,
		},
		{
			name: "G-06 Anticipation builder",
			uCtx: UserJourneyContext{
				Topic:          "Japan Trip",
				Event:          "Departure Flight",
				DaysUntilEvent: 4,
			},
			expectedStyle: StyleAnticipationBuilder,
		},
		{
			name: "G-07 Gentle nudge (absent user)",
			uCtx: UserJourneyContext{
				Topic:               "Sourdough Baking",
				DaysSinceLastActive: 7,
			},
			expectedStyle: StyleGentleNudge,
		},
		{
			name: "G-09 Empathetic acknowledgment",
			uCtx: UserJourneyContext{
				Topic:             "Tax Audit",
				SpecificChallenge: "reconciling 2024 receipts",
				HighStress:        true,
			},
			expectedStyle: StyleEmpatheticAck,
		},
		{
			name: "G-05 Invitation to continue",
			uCtx: UserJourneyContext{
				Topic:          "Go CLI",
				UnfinishedStep: "writing unit tests",
			},
			expectedStyle: StyleInvitationToContinue,
		},
		{
			name: "G-10 Low-pressure opener",
			uCtx: UserJourneyContext{
				Topic:            "Watercolor painting",
				LowPressureHobby: true,
			},
			expectedStyle: StyleLowPressureOpener,
		},
		{
			name: "G-08 Curiosity spark",
			uCtx: UserJourneyContext{
				Topic:          "Urban Farming",
				CuriosityAngle: "hydroponic nutrient cycles",
			},
			expectedStyle: StyleCuriositySpark,
		},
		{
			name: "G-03 Reflection prompt",
			uCtx: UserJourneyContext{
				Topic:     "Career pivot",
				TurnCount: 8,
			},
			expectedStyle: StyleReflectionPrompt,
		},
		{
			name: "G-01 Thoughtful check-in",
			uCtx: UserJourneyContext{
				Topic:    "Marathon training",
				Decision: "choosing running shoes",
			},
			expectedStyle: StyleThoughtfulCheckIn,
		},
	}

	for _, sc := range testScenarios {
		t.Run(sc.name, func(t *testing.T) {
			res, err := eng.Generate(ctx, sc.uCtx, nil)
			if err != nil {
				t.Fatalf("Generate failed: %v", err)
			}
			if res.StyleID != sc.expectedStyle {
				t.Errorf("expected style %s, got %s", sc.expectedStyle, res.StyleID)
			}
			if err := ValidateHeadline(res.Headline); err != nil {
				t.Errorf("generated invalid headline %q: %v", res.Headline, err)
			}
			if strings.Contains(res.CleanText, "[") || strings.Contains(res.CleanText, "]") {
				t.Errorf("unresolved tokens in clean text: %s", res.CleanText)
			}
		})
	}
}

func TestEngine_HistorySuppression(t *testing.T) {
	eng := NewEngine()
	ctx := context.Background()

	uCtx := UserJourneyContext{
		Topic:             "Project Dendrite",
		Milestone:         "Beta Release",
		MilestoneAchieved: true,
	}

	// First run should pick G-02
	res1, err := eng.Generate(ctx, uCtx, nil)
	if err != nil {
		t.Fatalf("run 1 failed: %v", err)
	}
	if res1.StyleID != StyleProgressCelebration {
		t.Errorf("expected G-02, got %s", res1.StyleID)
	}

	// Suppressing G-02 must yield another valid style
	res2, err := eng.Generate(ctx, uCtx, []StyleID{StyleProgressCelebration})
	if err != nil {
		t.Fatalf("run 2 failed: %v", err)
	}
	if res2.StyleID == StyleProgressCelebration {
		t.Errorf("expected G-02 to be suppressed, but got %s", res2.StyleID)
	}

	// Suppressing all candidate styles gracefully falls back
	allStyles := []StyleID{
		StyleThoughtfulCheckIn, StyleProgressCelebration, StyleReflectionPrompt,
		StyleCallbackToDetail, StyleInvitationToContinue, StyleAnticipationBuilder,
		StyleGentleNudge, StyleCuriositySpark, StyleEmpatheticAck, StyleLowPressureOpener,
	}
	resFallback, err := eng.Generate(ctx, uCtx, allStyles)
	if err != nil {
		t.Fatalf("fallback generation failed: %v", err)
	}
	if resFallback == nil || resFallback.Headline == "" {
		t.Errorf("expected valid fallback response")
	}
}

func TestEngine_VariableSubstitutionFallbacks(t *testing.T) {
	eng := NewEngine()
	ctx := context.Background()

	// Empty context should substitute safe defaults without error
	uCtx := UserJourneyContext{}
	res, err := eng.Generate(ctx, uCtx, nil)
	if err != nil {
		t.Fatalf("unexpected generation error on empty context: %v", err)
	}
	if strings.Contains(res.Headline, "[") {
		t.Errorf("unresolved brackets found in headline: %s", res.Headline)
	}
	if err := ValidateHeadline(res.Headline); err != nil {
		t.Errorf("headline failed validation: %v", err)
	}
}

func TestEngine_RegisterCustomStyle(t *testing.T) {
	eng := NewEngine()
	customID := StyleID("G-CUSTOM")
	eng.RegisterStyle(StyleDefinition{
		ID:             customID,
		Name:           "Custom style",
		OptimalContext: "Custom test trigger",
		Templates: []string{
			"Testing custom greeting for [topic].",
		},
		EvaluateScore: func(ctx UserJourneyContext) int {
			return 200 // Always wins
		},
	})

	res, err := eng.Generate(context.Background(), UserJourneyContext{Topic: "Custom Engine"}, nil)
	if err != nil {
		t.Fatalf("generation failed: %v", err)
	}
	if res.StyleID != customID {
		t.Errorf("expected custom style %s, got %s", customID, res.StyleID)
	}
}
