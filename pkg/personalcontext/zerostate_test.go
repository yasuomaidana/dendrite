package personalcontext

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestZeroStateEngine_Render_AllGreetingStyles(t *testing.T) {
	styles := []GreetingStyle{
		StyleThoughtfulCheckin,
		StyleProgressCelebration,
		StyleReflectionPrompt,
		StyleCallbackToDetail,
		StyleInvitationToContinue,
		StyleAnticipationBuilder,
		StyleGentleNudge,
		StyleMilestoneRecap,
		StyleStrategicPause,
		StyleCreativeSpark,
		GreetingStyle("unknown_fallback"),
	}

	engine := NewZeroStateEngine()

	for _, style := range styles {
		t.Run(string(style), func(t *testing.T) {
			req := ZeroStateRequest{
				Topic:         "Tokyo Trip 2027",
				GreetingStyle: style,
				Summary:       "Flights are booked and hotel search is underway.",
				NextSteps: []string{
					"Compare hotel options near Shinjuku",
					"Review rail pass pricing",
				},
				MemorySections: []MemorySection{
					{
						Emoji: "🏨",
						Title: "Accommodations",
						Facts: []string{"Shinjuku preference", "Budget $200/night"},
					},
					{
						Emoji: "",
						Title: "Transit",
						Facts: []string{"JR Pass under review"},
					},
				},
			}

			out, err := engine.Render(req)
			if err != nil {
				t.Fatalf("unexpected render error for style %s: %v", style, err)
			}

			if !strings.HasPrefix(out, "# ") {
				t.Errorf("expected output to start with '# ', got %q", out[:10])
			}

			if err := engine.Validate(out); err != nil {
				t.Fatalf("validation failed for valid output: %v", err)
			}
		})
	}
}

func TestZeroStateEngine_Render_BannedCliches(t *testing.T) {
	engine := NewZeroStateEngine()

	for _, cliche := range BannedCliches {
		t.Run(cliche, func(t *testing.T) {
			req := ZeroStateRequest{
				Topic:    "Home Renovation",
				Greeting: cliche + " start working on the kitchen?",
			}

			_, err := engine.Render(req)
			if err == nil {
				t.Fatalf("expected error for banned cliché %q, got nil", cliche)
			}
			if !errors.Is(err, ErrBannedCliche) {
				t.Errorf("expected ErrBannedCliche, got %v", err)
			}
		})
	}
}

func TestZeroStateEngine_Render_NextStepsRules(t *testing.T) {
	engine := NewZeroStateEngine()

	t.Run("too many next steps (>2)", func(t *testing.T) {
		req := ZeroStateRequest{
			Topic: "Alpha",
			NextSteps: []string{
				"Review item 1",
				"Draft item 2",
				"Compare item 3",
			},
		}
		_, err := engine.Render(req)
		if !errors.Is(err, ErrTooManyNextSteps) {
			t.Errorf("expected ErrTooManyNextSteps, got %v", err)
		}
	})

	t.Run("next step too long (>75 chars)", func(t *testing.T) {
		req := ZeroStateRequest{
			Topic: "Alpha",
			NextSteps: []string{
				"Review this extremely long description that has way too many words and details in it to exceed seventy-five characters total",
			},
		}
		_, err := engine.Render(req)
		if !errors.Is(err, ErrNextStepTooLong) {
			t.Errorf("expected ErrNextStepTooLong, got %v", err)
		}
	})

	t.Run("missing action verb", func(t *testing.T) {
		req := ZeroStateRequest{
			Topic: "Alpha",
			NextSteps: []string{
				"Tokyo tower tickets tomorrow",
			},
		}
		_, err := engine.Render(req)
		if !errors.Is(err, ErrNextStepActionVerb) {
			t.Errorf("expected ErrNextStepActionVerb, got %v", err)
		}
	})

	t.Run("two word action verb (walk through)", func(t *testing.T) {
		req := ZeroStateRequest{
			Topic: "Alpha",
			NextSteps: []string{
				"Walk through the daily itinerary",
			},
		}
		out, err := engine.Render(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "- Walk through the daily itinerary") {
			t.Errorf("expected rendered next step, got: %s", out)
		}
	})

	t.Run("ambiguous context omits next steps", func(t *testing.T) {
		req := ZeroStateRequest{
			Topic:            "Alpha",
			AmbiguousContext: true,
			NextSteps: []string{
				"Review options",
			},
		}
		out, err := engine.Render(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(out, "## Suggested next steps") {
			t.Errorf("expected next steps to be omitted for ambiguous context")
		}
	})
}

func TestZeroStateEngine_Render_TopicMemoryFallbacks(t *testing.T) {
	engine := NewZeroStateEngine()

	t.Run("raw memory string", func(t *testing.T) {
		req := ZeroStateRequest{
			Topic:     "Beta",
			RawMemory: "Custom raw memory notes here.",
		}
		out, err := engine.Render(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "Custom raw memory notes here.") {
			t.Errorf("expected raw memory to be present in output")
		}
	})

	t.Run("empty memory default", func(t *testing.T) {
		req := ZeroStateRequest{
			Topic: "Gamma",
		}
		out, err := engine.Render(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "📌 Initial context for Gamma.") {
			t.Errorf("expected default memory context")
		}
	})
}

func TestZeroStateEngine_Validate(t *testing.T) {
	engine := NewZeroStateEngine()

	t.Run("too short", func(t *testing.T) {
		err := engine.Validate("hello\nworld")
		if err == nil || !strings.Contains(err.Error(), "too short") {
			t.Errorf("expected too short error, got %v", err)
		}
	})

	t.Run("invalid greeting prefix", func(t *testing.T) {
		raw := "Greeting without hash\n\nSummary\n\n\n\n\n\n```\nTopic memory\n```"
		err := engine.Validate(raw)
		if !errors.Is(err, ErrInvalidGreeting) {
			t.Errorf("expected ErrInvalidGreeting, got %v", err)
		}
	})

	t.Run("banned cliche greeting", func(t *testing.T) {
		raw := "# Ready to plan the trip?\n\nSummary\n\n\n\n\n\n```\nTopic memory\n```"
		err := engine.Validate(raw)
		if !errors.Is(err, ErrBannedCliche) {
			t.Errorf("expected ErrBannedCliche, got %v", err)
		}
	})

	t.Run("missing topic memory block", func(t *testing.T) {
		raw := "# Heading\n\nSummary\n\n\n\n\n\nSome other notes\n"
		err := engine.Validate(raw)
		if !errors.Is(err, ErrMissingTopicMemory) {
			t.Errorf("expected ErrMissingTopicMemory, got %v", err)
		}
	})

	t.Run("invalid spacing", func(t *testing.T) {
		raw := "# Heading\n\nSummary\n\n```\nTopic memory\n\nNotes\n```"
		err := engine.Validate(raw)
		if !errors.Is(err, ErrInvalidSpacing) {
			t.Errorf("expected ErrInvalidSpacing, got %v", err)
		}
	})

	t.Run("validate next steps with formatting errors", func(t *testing.T) {
		// next step with asterisk bullet
		validAsterisk := "# Valid Heading\n\nSummary\n\n## Suggested next steps, in bullet points\n\n* Review plan\n\n\n\n\n\n```\nTopic memory\n\nNotes\n```"
		if err := engine.Validate(validAsterisk); err != nil {
			t.Fatalf("unexpected validation error on asterisk bullet: %v", err)
		}

		// next step too long
		longStep := strings.Repeat("A", 80)
		bad1 := "# Valid Heading\n\nSummary\n\n## Suggested next steps, in bullet points\n\n- " + longStep + "\n\n\n\n\n\n```\nTopic memory\n\nNotes\n```"
		if err := engine.Validate(bad1); !errors.Is(err, ErrNextStepTooLong) {
			t.Errorf("expected ErrNextStepTooLong, got %v", err)
		}

		// next step missing action verb
		bad2 := "# Valid Heading\n\nSummary\n\n## Suggested next steps, in bullet points\n\n- Tokyo tomorrow\n\n\n\n\n\n```\nTopic memory\n\nNotes\n```"
		if err := engine.Validate(bad2); !errors.Is(err, ErrNextStepActionVerb) {
			t.Errorf("expected ErrNextStepActionVerb, got %v", err)
		}

		// too many next steps
		bad3 := "# Valid Heading\n\nSummary\n\n## Suggested next steps, in bullet points\n\n- Review 1\n- Compare 2\n- Draft 3\n\n\n\n\n\n```\nTopic memory\n\nNotes\n```"
		if err := engine.Validate(bad3); !errors.Is(err, ErrTooManyNextSteps) {
			t.Errorf("expected ErrTooManyNextSteps, got %v", err)
		}

		// spacing appears before next steps header inside code block
		bad5 := "# Valid Heading\n\nSummary\n\n\n\n\n\n```\nTopic memory\n## Suggested next steps, in bullet points\n- Review 1\n```"
		if err := engine.Validate(bad5); !errors.Is(err, ErrInvalidSpacing) {
			t.Errorf("expected ErrInvalidSpacing when 5 newlines precede next steps header, got %v", err)
		}
	})
}

func TestMultiTurnStateMachine(t *testing.T) {
	ctx := context.Background()
	engine := NewZeroStateEngine()
	initialReq := ZeroStateRequest{
		Topic:         "Summer Vacation",
		GreetingStyle: StyleThoughtfulCheckin,
		Summary:       "Initial flight details saved.",
		NextSteps: []string{
			"Review hotel deals",
		},
	}

	sm := NewMultiTurnStateMachine(engine, initialReq)

	if sm.CurrentTurn() != 0 {
		t.Errorf("expected initial turn 0, got %d", sm.CurrentTurn())
	}
	if sm.CurrentState() != Turn1ZeroState {
		t.Errorf("expected Turn1ZeroState, got %s", sm.CurrentState())
	}

	// Turn 1: Zero State
	out1, err := sm.ProcessTurn(ctx, TurnInput{UserMessage: "hello"})
	if err != nil {
		t.Fatalf("turn 1 error: %v", err)
	}
	if out1.Turn != 1 || !out1.IsZeroState || out1.State != Turn1ZeroState {
		t.Errorf("unexpected turn 1 output: %+v", out1)
	}
	if !strings.HasPrefix(out1.Content, "# ") {
		t.Errorf("turn 1 must start with '# ', got %q", out1.Content[:10])
	}

	// Turn 2: Normal Chat
	out2, err := sm.ProcessTurn(ctx, TurnInput{UserMessage: "Can you tell me about the weather there?"})
	if err != nil {
		t.Fatalf("turn 2 error: %v", err)
	}
	if out2.Turn != 2 || out2.IsZeroState || out2.State != TurnNNormalChat {
		t.Errorf("unexpected turn 2 output: %+v", out2)
	}
	if strings.Contains(out2.Content, "# ") || strings.Contains(out2.Content, "Topic memory") {
		t.Errorf("turn 2 must not regurgitate scaffolding: %s", out2.Content)
	}

	// Turn 3: Correction Trigger
	updatedReq := ZeroStateRequest{
		Topic:   "Summer Vacation",
		Summary: "Trip canceled, pivoting to staycation.",
	}
	out3, err := sm.ProcessTurn(ctx, TurnInput{
		UserMessage:   "Actually, I canceled that trip and want a staycation.",
		UpdatedMemory: &updatedReq,
	})
	if err != nil {
		t.Fatalf("turn 3 error: %v", err)
	}
	if out3.Turn != 3 || !out3.IsZeroState || out3.State != StateCorrection {
		t.Errorf("unexpected turn 3 output: %+v", out3)
	}
	if !strings.Contains(out3.Content, "Got it, I've updated the topic memory to reflect that.") {
		t.Errorf("expected acknowledgment sentence in turn 3: %s", out3.Content)
	}
	if !strings.Contains(out3.Content, "staycation") {
		t.Errorf("expected updated memory reflected in regenerated Zero State")
	}

	// Reset with new request
	newReq := ZeroStateRequest{Topic: "Winter Ski"}
	sm.Reset(&newReq)
	if sm.CurrentTurn() != 0 || sm.CurrentState() != Turn1ZeroState {
		t.Errorf("reset failed")
	}
	if sm.currentReq.Topic != "Winter Ski" {
		t.Errorf("reset did not adopt new request")
	}

	// Reset with nil request
	sm.Reset(nil)
	if sm.CurrentTurn() != 0 {
		t.Errorf("reset with nil failed")
	}
}

func TestMultiTurnStateMachine_ErrorsAndTriggers(t *testing.T) {
	ctx := context.Background()
	engine := NewZeroStateEngine()

	t.Run("turn 1 render error", func(t *testing.T) {
		badReq := ZeroStateRequest{
			Greeting: "Ready to start?", // banned cliché
		}
		sm := NewMultiTurnStateMachine(engine, badReq)
		_, err := sm.ProcessTurn(ctx, TurnInput{UserMessage: "hi"})
		if !errors.Is(err, ErrBannedCliche) {
			t.Errorf("expected ErrBannedCliche, got %v", err)
		}
	})

	t.Run("turn 2 normal chat violation in ProcessTurn", func(t *testing.T) {
		badTopicReq := ZeroStateRequest{
			Topic: "\n# Malicious Headline In Topic",
		}
		sm := NewMultiTurnStateMachine(engine, badTopicReq)
		_, err := sm.ProcessTurn(ctx, TurnInput{UserMessage: "turn 1"})
		if err != nil {
			t.Fatalf("turn 1 failed: %v", err)
		}

		// Turn 2 should fail validation because the topic injected a markdown headline
		_, err = sm.ProcessTurn(ctx, TurnInput{UserMessage: "turn 2"})
		if !errors.Is(err, ErrTurnViolation) {
			t.Errorf("expected ErrTurnViolation in turn 2, got %v", err)
		}
	})

	t.Run("correction render error", func(t *testing.T) {
		goodReq := ZeroStateRequest{Topic: "OK"}
		sm := NewMultiTurnStateMachine(engine, goodReq)
		_, _ = sm.ProcessTurn(ctx, TurnInput{UserMessage: "hi"}) // turn 1 pass

		badUpdate := ZeroStateRequest{Greeting: "Want to do this?"}
		_, err := sm.ProcessTurn(ctx, TurnInput{
			UserMessage:   "Actually change the plan",
			UpdatedMemory: &badUpdate,
		})
		if !errors.Is(err, ErrBannedCliche) {
			t.Errorf("expected ErrBannedCliche on correction, got %v", err)
		}
	})

	t.Run("validateNormalChat violations", func(t *testing.T) {
		sm := NewMultiTurnStateMachine(nil, ZeroStateRequest{})

		if err := sm.validateNormalChat("# Heading\nBody"); err == nil {
			t.Errorf("expected error for header in normal chat")
		}
		if err := sm.validateNormalChat("## Suggested next steps, in bullet points\nBody"); err == nil {
			t.Errorf("expected error for next steps in normal chat")
		}
		if err := sm.validateNormalChat("Topic memory block here"); err == nil {
			t.Errorf("expected error for topic memory in normal chat")
		}
	})

	t.Run("correction trigger keywords", func(t *testing.T) {
		sm := NewMultiTurnStateMachine(nil, ZeroStateRequest{})
		triggers := []string{
			"Actually, change this",
			"actually I want that",
			"Please update my contact email",
			"Update the address",
			"Change my booking",
			"Change the hotel",
			"Correction: it is 3pm not 4pm",
			"I canceled the order",
			"I cancelled the reservation",
			"I decided to change my mind",
			"Scratch that idea",
		}
		for _, tr := range triggers {
			if !sm.IsCorrectionTrigger(tr) {
				t.Errorf("expected IsCorrectionTrigger to be true for: %q", tr)
			}
		}

		if sm.IsCorrectionTrigger("Tell me more about restaurants in town.") {
			t.Errorf("regular message should not trigger correction")
		}
	})

	t.Run("empty step action verb helper", func(t *testing.T) {
		if engine.hasActionVerbPrefix("") {
			t.Errorf("empty string should not have action verb prefix")
		}
	})
}
