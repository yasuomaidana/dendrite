package memory

import (
	"errors"
	"strings"
	"testing"
)

func TestParser_Parse_ValidBlock(t *testing.T) {
	raw := "```\n" + MandatoryDisclaimer + "\n\n" +
		"🏨 Accommodations\n" +
		"  - Shinjuku hotel preference\n" +
		"  * Budget $200 per night\n\n" +
		"✈️ Flights\n" +
		"  - Delta flight booked for 2027\n" +
		"    Terminal 2 arrival\n" +
		"```"

	p := NewParser()
	mb, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if len(mb.Sections) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(mb.Sections))
	}

	if mb.Sections[0].Emoji != "🏨" || mb.Sections[0].Title != "Accommodations" {
		t.Errorf("unexpected first section: %+v", mb.Sections[0])
	}
	if len(mb.Sections[0].Facts) != 2 {
		t.Errorf("expected 2 facts in first section, got %d", len(mb.Sections[0].Facts))
	}

	if mb.Sections[1].Emoji != "✈️" || mb.Sections[1].Title != "Flights" {
		t.Errorf("unexpected second section: %+v", mb.Sections[1])
	}
	if len(mb.Sections[1].Facts) != 2 {
		t.Errorf("expected 2 facts in second section, got %d", len(mb.Sections[1].Facts))
	}
}

func TestParser_Parse_Errors(t *testing.T) {
	p := NewParser()

	t.Run("missing backticks", func(t *testing.T) {
		_, err := p.Parse("Topic memory\nno backticks")
		if !errors.Is(err, ErrMalformedBlock) {
			t.Errorf("expected ErrMalformedBlock, got %v", err)
		}
	})

	t.Run("missing topic memory header", func(t *testing.T) {
		raw := "```\nRandom text here\n```"
		_, err := p.Parse(raw)
		if !errors.Is(err, ErrMissingDisclaimer) {
			t.Errorf("expected ErrMissingDisclaimer, got %v", err)
		}
	})

	t.Run("altered disclaimer text", func(t *testing.T) {
		raw := "```\nTopic memory\nAltered disclaimer lines\n🏨 Hotel\n - Room\n```"
		_, err := p.Parse(raw)
		if !errors.Is(err, ErrMissingDisclaimer) {
			t.Errorf("expected ErrMissingDisclaimer, got %v", err)
		}
	})

	t.Run("fact before section", func(t *testing.T) {
		raw := "```\n" + MandatoryDisclaimer + "\n\n- Fact before any section\n🏨 Hotel\n```"
		_, err := p.Parse(raw)
		if !errors.Is(err, ErrMalformedBlock) {
			t.Errorf("expected ErrMalformedBlock, got %v", err)
		}
	})

	t.Run("section missing emoji", func(t *testing.T) {
		raw := "```\n" + MandatoryDisclaimer + "\n\nPlain Section Without Emoji\n- Fact\n```"
		_, err := p.Parse(raw)
		if !errors.Is(err, ErrMissingEmojiHeader) {
			t.Errorf("expected ErrMissingEmojiHeader, got %v", err)
		}
	})

	t.Run("no sections found", func(t *testing.T) {
		raw := "```\n" + MandatoryDisclaimer + "\n\n```"
		_, err := p.Parse(raw)
		if !errors.Is(err, ErrNoSectionsFound) {
			t.Errorf("expected ErrNoSectionsFound, got %v", err)
		}
	})
}

func TestParser_Format_AndWrapping(t *testing.T) {
	p := NewParser()
	mb := &MemoryBlock{
		Disclaimer: MandatoryDisclaimer,
		Sections: []Section{
			{
				Emoji: "🏨",
				Title: "Accommodations",
				Facts: []string{
					"Shinjuku hotel near station",
					"This is an extraordinarily long sentence designed specifically to trigger the word wrapping logic in Format to ensure it wraps cleanly across multiple lines without exceeding max column width",
				},
			},
			{
				Emoji: "✈️",
				Title: "Flights",
				Facts: []string{"Flight booked"},
			},
		},
	}

	formatted := p.Format(mb, 50)
	if !strings.HasPrefix(formatted, "```\n") || !strings.HasSuffix(formatted, "```") {
		t.Errorf("formatted text missing outer fences")
	}
	if !strings.Contains(formatted, "🏨 Accommodations") {
		t.Errorf("formatted text missing section")
	}

	// Test default maxLineWidth <= 0
	formattedDefault := p.Format(mb, 0)
	if len(formattedDefault) == 0 {
		t.Errorf("expected non-empty output with default maxLineWidth")
	}
}

func TestFidelityGuard_Verify(t *testing.T) {
	guard := NewFidelityGuard()

	originalContext := "User is traveling to Tokyo in 2027. Looking for a hotel in Shinjuku with a budget of $200 per night. Flying on Delta."

	mb := &MemoryBlock{
		Disclaimer: MandatoryDisclaimer,
		Sections: []Section{
			{
				Emoji: "🏨",
				Title: "Accommodations",
				Facts: []string{
					"Tokyo hotel in Shinjuku",
					"Budget $200 per night",
				},
			},
			{
				Emoji: "✈️",
				Title: "Transit",
				Facts: []string{
					"Flying on Delta in 2027",
				},
			},
		},
	}

	t.Run("lossless grounded memory passes", func(t *testing.T) {
		rep, err := guard.Verify(originalContext, mb, FidelityGuardOptions{})
		if err != nil {
			t.Fatalf("unexpected fidelity error: %v", err)
		}
		if !rep.Passed || rep.FidelityRatio < 1.0 {
			t.Errorf("expected 100%% fidelity pass, got %+v", rep)
		}
	})

	t.Run("extrapolated term fails", func(t *testing.T) {
		extrapolatedMB := &MemoryBlock{
			Disclaimer: MandatoryDisclaimer,
			Sections: []Section{
				{
					Emoji: "🏨",
					Title: "Accommodations",
					Facts: []string{
						"Tokyo hotel in Shinjuku",
						"Budget $200 per night",
						"Luxury 5-star ryokan in Gion with private onsen spa",
					},
				},
				{
					Emoji: "✈️",
					Title: "Transit",
					Facts: []string{"Flying on Delta in 2027"},
				},
			},
		}

		rep, err := guard.Verify(originalContext, extrapolatedMB, FidelityGuardOptions{})
		if err == nil {
			t.Fatalf("expected ErrExtrapolationDetected, got nil")
		}
		if !errors.Is(err, ErrExtrapolationDetected) {
			t.Errorf("expected ErrExtrapolationDetected, got %v", err)
		}
		if rep.Passed {
			t.Errorf("report should indicate failure")
		}
	})

	t.Run("missing key entity fails", func(t *testing.T) {
		missingMB := &MemoryBlock{
			Disclaimer: MandatoryDisclaimer,
			Sections: []Section{
				{
					Emoji: "🏨",
					Title: "Accommodations",
					Facts: []string{
						"Tokyo hotel in Shinjuku",
						// $200 missing
					},
				},
				{
					Emoji: "✈️",
					Title: "Transit",
					Facts: []string{"Flying on Delta in 2027"},
				},
			},
		}

		_, err := guard.Verify(originalContext, missingMB, FidelityGuardOptions{})
		if err == nil {
			t.Fatalf("expected ErrInformationLossDetected, got nil")
		}
		if !errors.Is(err, ErrInformationLossDetected) {
			t.Errorf("expected ErrInformationLossDetected, got %v", err)
		}
	})

	t.Run("nil or empty memory block", func(t *testing.T) {
		_, err := guard.Verify(originalContext, nil, FidelityGuardOptions{})
		if !errors.Is(err, ErrNoSectionsFound) {
			t.Errorf("expected ErrNoSectionsFound for nil block, got %v", err)
		}

		_, err = guard.Verify(originalContext, &MemoryBlock{}, FidelityGuardOptions{})
		if !errors.Is(err, ErrNoSectionsFound) {
			t.Errorf("expected ErrNoSectionsFound for empty block, got %v", err)
		}
	})
}
