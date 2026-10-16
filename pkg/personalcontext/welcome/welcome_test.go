package welcome

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestDispatcher_SelectPersona(t *testing.T) {
	d := NewDispatcher()

	t.Run("empty items selects trustworthy guide", func(t *testing.T) {
		p := d.SelectPersona(nil)
		if p != PersonaTrustworthyGuide {
			t.Errorf("expected %s, got %s", PersonaTrustworthyGuide, p)
		}
	})

	t.Run("only permissions source selects trustworthy guide", func(t *testing.T) {
		items := []DataItem{
			{Source: SourcePermissions, Title: "Location Access"},
		}
		p := d.SelectPersona(items)
		if p != PersonaTrustworthyGuide {
			t.Errorf("expected %s, got %s", PersonaTrustworthyGuide, p)
		}
	})

	t.Run("trifecta domains selects showcase of possibilities", func(t *testing.T) {
		items := []DataItem{
			{Source: SourceCalendar, Category: "productivity", Title: "Sprint Planning"},
			{Source: SourcePhotos, Category: "passion", Title: "Pottery Workshop"},
			{Source: SourceSearchHistory, Category: "curiosity", Title: "Quantum Computing"},
		}
		p := d.SelectPersona(items)
		if p != PersonaShowcasePossibilities {
			t.Errorf("expected %s, got %s", PersonaShowcasePossibilities, p)
		}
	})

	t.Run("multi-source selects insightful synthesizer", func(t *testing.T) {
		items := []DataItem{
			{Source: SourceGmail, Title: "Austin Trip"},
			{Source: SourcePhotos, Title: "BBQ Photos"},
		}
		p := d.SelectPersona(items)
		if p != PersonaInsightfulSynthesizer {
			t.Errorf("expected %s, got %s", PersonaInsightfulSynthesizer, p)
		}
	})

	t.Run("creative source selects creative muse", func(t *testing.T) {
		items := []DataItem{
			{Source: SourcePhotos, Title: "Sunset Shots", Category: "passion"},
		}
		p := d.SelectPersona(items)
		if p != PersonaCreativeMuse {
			t.Errorf("expected %s, got %s", PersonaCreativeMuse, p)
		}
	})

	t.Run("single productivity source selects productivity partner", func(t *testing.T) {
		items := []DataItem{
			{Source: SourceCalendar, Title: "Team Meeting"},
		}
		p := d.SelectPersona(items)
		if p != PersonaProductivityPartner {
			t.Errorf("expected %s, got %s", PersonaProductivityPartner, p)
		}
	})
}

func TestDispatcher_Synthesize_ProductivityPartner(t *testing.T) {
	d := NewDispatcher()
	ctx := context.Background()

	t.Run("logistics with flight number stripping and limit to 3", func(t *testing.T) {
		items := []DataItem{
			{Source: SourceGmail, Title: "Booking UA 1234 to Chicago"},
			{Source: SourceCalendar, Title: "Client Dinner"},
			{Source: SourceTasks, Title: "Prepare presentation"},
			{Source: SourceCalendar, Title: "Extra fourth item"},
		}

		resp, err := d.Synthesize(ctx, SynthesisRequest{
			Items:         items,
			ForcedPersona: PersonaProductivityPartner,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if resp.Persona != PersonaProductivityPartner {
			t.Errorf("expected %s, got %s", PersonaProductivityPartner, resp.Persona)
		}
		if strings.Contains(resp.Body, "UA 1234") {
			t.Errorf("trivial flight number was not stripped: %s", resp.Body)
		}
		if len(resp.Actions) != 3 {
			t.Errorf("expected 3 actions, got %d", len(resp.Actions))
		}
		if len(resp.Sources) == 0 {
			t.Errorf("expected non-empty sources")
		}
	})

	t.Run("empty items fallback", func(t *testing.T) {
		resp, err := d.Synthesize(ctx, SynthesisRequest{
			Items:         nil,
			ForcedPersona: PersonaProductivityPartner,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(resp.Body, "upcoming schedule and tasks") {
			t.Errorf("expected fallback text in body, got: %s", resp.Body)
		}
	})

	t.Run("automatic persona selection when ForcedPersona is empty", func(t *testing.T) {
		items := []DataItem{
			{Source: SourceCalendar, Title: "Planning session"},
		}
		resp, err := d.Synthesize(ctx, SynthesisRequest{
			Items: items,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Persona != PersonaProductivityPartner {
			t.Errorf("expected %s, got %s", PersonaProductivityPartner, resp.Persona)
		}
	})
}

func TestDispatcher_Synthesize_CreativeMuse(t *testing.T) {
	d := NewDispatcher()
	ctx := context.Background()

	t.Run("explicit passion category", func(t *testing.T) {
		items := []DataItem{
			{Source: SourcePhotos, Category: "passion", Title: "Sourdough Bread"},
		}
		resp, err := d.Synthesize(ctx, SynthesisRequest{
			Items:         items,
			ForcedPersona: PersonaCreativeMuse,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(resp.Greeting, "Sourdough Bread") {
			t.Errorf("expected passion title in greeting, got: %s", resp.Greeting)
		}
	})

	t.Run("fallback to first item title", func(t *testing.T) {
		items := []DataItem{
			{Source: SourcePhotos, Title: "Woodworking"},
		}
		resp, err := d.Synthesize(ctx, SynthesisRequest{
			Items:         items,
			ForcedPersona: PersonaCreativeMuse,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(resp.Greeting, "Woodworking") {
			t.Errorf("expected first item title in greeting, got: %s", resp.Greeting)
		}
	})

	t.Run("fallback to default creative projects", func(t *testing.T) {
		resp, err := d.Synthesize(ctx, SynthesisRequest{
			Items:         nil,
			ForcedPersona: PersonaCreativeMuse,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(resp.Greeting, "your creative projects") {
			t.Errorf("expected default creative projects text, got: %s", resp.Greeting)
		}
	})
}

func TestDispatcher_Synthesize_TrustworthyGuide(t *testing.T) {
	d := NewDispatcher()
	ctx := context.Background()

	t.Run("with specific user interest", func(t *testing.T) {
		resp, err := d.Synthesize(ctx, SynthesisRequest{
			ForcedPersona: PersonaTrustworthyGuide,
			UserInterest:  "NASA Mars Rover",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(resp.Body, "Because you follow NASA Mars Rover") {
			t.Errorf("expected customized hook with user interest, got: %s", resp.Body)
		}
		if !strings.Contains(resp.Body, "Q: How will my data be used?") {
			t.Errorf("missing mandatory Q&A in body")
		}
	})

	t.Run("default hook when interest is empty", func(t *testing.T) {
		resp, err := d.Synthesize(ctx, SynthesisRequest{
			ForcedPersona: PersonaTrustworthyGuide,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(resp.Body, "Because you connect your personal context") {
			t.Errorf("expected generic hook, got: %s", resp.Body)
		}
	})
}

func TestDispatcher_Synthesize_InsightfulSynthesizer(t *testing.T) {
	d := NewDispatcher()
	ctx := context.Background()

	t.Run("successful synthesis with >= 2 sources", func(t *testing.T) {
		items := []DataItem{
			{Source: SourceGmail, Title: "Conference registration"},
			{Source: SourceCalendar, Title: "Flight to Seattle"},
		}
		resp, err := d.Synthesize(ctx, SynthesisRequest{
			Items:         items,
			ForcedPersona: PersonaInsightfulSynthesizer,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Persona != PersonaInsightfulSynthesizer {
			t.Errorf("expected %s, got %s", PersonaInsightfulSynthesizer, resp.Persona)
		}
		if !strings.Contains(resp.Body, "Conference registration and Flight to Seattle") {
			t.Errorf("expected combined topics, got: %s", resp.Body)
		}
	})

	t.Run("error when fewer than 2 distinct sources", func(t *testing.T) {
		items := []DataItem{
			{Source: SourceGmail, Title: "Single email item"},
		}
		_, err := d.Synthesize(ctx, SynthesisRequest{
			Items:         items,
			ForcedPersona: PersonaInsightfulSynthesizer,
		})
		if err == nil {
			t.Fatalf("expected error for single source, got nil")
		}
	})

	t.Run("mechanical exposition error detection", func(t *testing.T) {
		items := []DataItem{
			{Source: SourceGmail, Title: "I saw this in your email"},
			{Source: SourceCalendar, Title: "Meeting"},
		}
		_, err := d.Synthesize(ctx, SynthesisRequest{
			Items:         items,
			ForcedPersona: PersonaInsightfulSynthesizer,
		})
		if !errors.Is(err, ErrMechanicalExposition) {
			t.Errorf("expected ErrMechanicalExposition, got %v", err)
		}
	})
}

func TestDispatcher_Synthesize_ShowcasePossibilities(t *testing.T) {
	d := NewDispatcher()
	ctx := context.Background()

	t.Run("successful trifecta generation", func(t *testing.T) {
		items := []DataItem{
			{Source: SourceCalendar, Category: "productivity", Title: "Product Roadmap Review"},
			{Source: SourcePhotos, Category: "passion", Title: "Guitar Lessons"},
			{Source: SourceSearchHistory, Category: "curiosity", Title: "Astrophysics Podcast"},
		}
		resp, err := d.Synthesize(ctx, SynthesisRequest{
			Items:         items,
			ForcedPersona: PersonaShowcasePossibilities,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Persona != PersonaShowcasePossibilities {
			t.Errorf("expected %s, got %s", PersonaShowcasePossibilities, resp.Persona)
		}
		if !strings.Contains(resp.Body, "Which of these areas would you like to explore first?") {
			t.Errorf("expected mandatory closing choice CTA")
		}
		if len(resp.Actions) != 3 {
			t.Errorf("expected 3 trifecta actions, got %d", len(resp.Actions))
		}
	})

	t.Run("missing a domain returns error", func(t *testing.T) {
		items := []DataItem{
			{Source: SourceCalendar, Category: "productivity", Title: "Work"},
			{Source: SourcePhotos, Category: "passion", Title: "Hobby"},
			// curiosity missing
		}
		_, err := d.Synthesize(ctx, SynthesisRequest{
			Items:         items,
			ForcedPersona: PersonaShowcasePossibilities,
		})
		if !errors.Is(err, ErrMissingTrifectaDomain) {
			t.Errorf("expected ErrMissingTrifectaDomain, got %v", err)
		}
	})
}

func TestDispatcher_SensitivityAndFiltering(t *testing.T) {
	d := NewDispatcher()
	ctx := context.Background()

	items := []DataItem{
		{Source: SourceGmail, Title: "Work meeting", Sensitive: false},
		{Source: SourceGmail, Title: "Doctor appointment", Details: "Medical diagnosis and prescription results", Sensitive: false},
		{Source: SourceCalendar, Title: "Private Therapy", Sensitive: true},
	}

	resp, err := d.Synthesize(ctx, SynthesisRequest{
		Items:         items,
		ForcedPersona: PersonaProductivityPartner,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(resp.Body, "Medical diagnosis") || strings.Contains(resp.Body, "Private Therapy") {
		t.Errorf("invasive/sensitive data was not filtered out: %s", resp.Body)
	}
}

func TestDispatcher_DefaultPersonaBranch(t *testing.T) {
	d := NewDispatcher()
	ctx := context.Background()

	resp, err := d.Synthesize(ctx, SynthesisRequest{
		ForcedPersona: Persona("unknown_persona"),
	})
	if err != nil {
		t.Fatalf("unexpected error on unknown persona: %v", err)
	}
	if resp.Persona != PersonaProductivityPartner {
		t.Errorf("expected fallback to ProductivityPartner, got %s", resp.Persona)
	}
}
