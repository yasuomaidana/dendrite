package greeting

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"regexp"
	"strings"
	"time"
)

// Typed error definitions
var (
	ErrBannedCliche            = errors.New("greeting contains a banned cliché opener")
	ErrInvalidGreetingHeadline = errors.New("greeting must be a single-sentence headline starting with '# '")
	ErrUnresolvedTemplateToken = errors.New("greeting contains unresolved template tokens")
	ErrNoMatchingStyle         = errors.New("no matching greeting style found for context")
	ErrHeadlineTooLong         = errors.New("greeting headline exceeds maximum length of 120 characters")
)

// StyleID represents one of the 10 canonical dynamic greeting styles.
type StyleID string

const (
	StyleThoughtfulCheckIn    StyleID = "G-01" // Thoughtful check-in
	StyleProgressCelebration  StyleID = "G-02" // Progress celebration
	StyleReflectionPrompt     StyleID = "G-03" // Reflection prompt
	StyleCallbackToDetail     StyleID = "G-04" // Callback to specific detail
	StyleInvitationToContinue StyleID = "G-05" // Invitation to continue
	StyleAnticipationBuilder  StyleID = "G-06" // Anticipation builder
	StyleGentleNudge          StyleID = "G-07" // Gentle nudge
	StyleCuriositySpark       StyleID = "G-08" // Curiosity spark
	StyleEmpatheticAck        StyleID = "G-09" // Empathetic acknowledgment
	StyleLowPressureOpener    StyleID = "G-10" // Low-pressure opener
)

// BannedCliches lists prohibited openers and cliché phrases.
var BannedCliches = []string{
	"ready to",
	"let's",
	"lets",
	"time to",
	"looking to",
	"want to",
	"how about",
	"i hope this finds you well",
	"as an ai",
	"certainly",
	"sure thing",
	"great question",
	"let's dive in",
	"without further ado",
}

// UserJourneyContext provides factual signals about the user's ongoing journey.
type UserJourneyContext struct {
	Topic               string   `json:"topic"`
	Decision            string   `json:"decision,omitempty"`
	Milestone           string   `json:"milestone,omitempty"`
	Event               string   `json:"event,omitempty"`
	DaysUntilEvent      int      `json:"days_until_event,omitempty"`
	Options             []string `json:"options,omitempty"`
	UnfinishedStep      string   `json:"unfinished_step,omitempty"`
	SpecificChallenge   string   `json:"specific_challenge,omitempty"`
	CuriosityAngle      string   `json:"curiosity_angle,omitempty"`
	DaysSinceLastActive int      `json:"days_since_last_active,omitempty"`
	TurnCount           int      `json:"turn_count,omitempty"`
	MilestoneAchieved   bool     `json:"milestone_achieved,omitempty"`
	HighStress          bool     `json:"high_stress,omitempty"`
	LowPressureHobby    bool     `json:"low_pressure_hobby,omitempty"`
}

// StyleDefinition encapsulates metadata, templates, and trigger scoring for a style.
type StyleDefinition struct {
	ID             StyleID                          `json:"id"`
	Name           string                           `json:"name"`
	OptimalContext string                           `json:"optimal_context"`
	Templates      []string                         `json:"templates"`
	EvaluateScore  func(ctx UserJourneyContext) int `json:"-"`
}

// GeneratedGreeting represents the outcome of the greeting matrix resolution.
type GeneratedGreeting struct {
	StyleID        StyleID  `json:"style_id"`
	StyleName      string   `json:"style_name"`
	Headline       string   `json:"headline"`
	CleanText      string   `json:"clean_text"`
	TemplateUsed   string   `json:"template_used"`
	SuppressedHits []string `json:"suppressed_hits,omitempty"`
}

// Engine coordinates the 10-Style Greeting Matrix and Cliché Suppression Engine.
type Engine struct {
	styles       map[StyleID]StyleDefinition
	orderedIDs   []StyleID
	historyLimit int
	rng          *rand.Rand
}

// NewEngine constructs a pre-configured 10-Style Greeting Matrix engine.
func NewEngine() *Engine {
	e := &Engine{
		styles:       make(map[StyleID]StyleDefinition),
		orderedIDs:   make([]StyleID, 0, 10),
		historyLimit: 3,
		rng:          rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	e.registerCanonicalStyles()
	return e
}

func (e *Engine) registerCanonicalStyles() {
	e.RegisterStyle(StyleDefinition{
		ID:             StyleThoughtfulCheckIn,
		Name:           "Thoughtful check-in",
		OptimalContext: "Ongoing journey without recent major inflection points",
		Templates: []string{
			"How's [topic] coming along?",
			"Still feeling good about [decision]?",
			"Checking in on your progress with [topic].",
		},
		EvaluateScore: func(ctx UserJourneyContext) int {
			if ctx.Topic != "" && !ctx.MilestoneAchieved && ctx.DaysSinceLastActive < 5 && !ctx.HighStress {
				if ctx.Decision != "" {
					return 85
				}
				return 70
			}
			return 30
		},
	})

	e.RegisterStyle(StyleDefinition{
		ID:             StyleProgressCelebration,
		Name:           "Progress celebration",
		OptimalContext: "User achieved a milestone, finalized a purchase, or finished a task",
		Templates: []string{
			"That's a big one — [milestone] is done!",
			"Major progress on [topic] with [milestone] wrapped up.",
			"Milestone reached: [milestone] is complete!",
		},
		EvaluateScore: func(ctx UserJourneyContext) int {
			if ctx.MilestoneAchieved && ctx.Milestone != "" {
				return 100
			}
			return 0
		},
	})

	e.RegisterStyle(StyleDefinition{
		ID:             StyleReflectionPrompt,
		Name:           "Reflection prompt",
		OptimalContext: "Long-running journey where high-level review is valuable",
		Templates: []string{
			"A lot has happened with [topic] — worth a quick look back?",
			"Looking across your recent steps on [topic], ready to reflect on what's next?",
			"Reflecting on the path so far with [topic].",
		},
		EvaluateScore: func(ctx UserJourneyContext) int {
			if ctx.TurnCount >= 5 && ctx.Topic != "" && !ctx.MilestoneAchieved {
				return 80
			}
			return 20
		},
	})

	e.RegisterStyle(StyleDefinition{
		ID:             StyleCallbackToDetail,
		Name:           "Callback to specific detail",
		OptimalContext: "Memorable key decision point or fork in the road",
		Templates: []string{
			"You were weighing [Option A vs B] — any verdict?",
			"Back to that choice between [Option A vs B] — how are you leaning?",
			"Following up on the decision between [Option A vs B].",
		},
		EvaluateScore: func(ctx UserJourneyContext) int {
			if len(ctx.Options) >= 2 {
				return 95
			}
			return 0
		},
	})

	e.RegisterStyle(StyleDefinition{
		ID:             StyleInvitationToContinue,
		Name:           "Invitation to continue",
		OptimalContext: "Clear unfinished task waiting from previous turn",
		Templates: []string{
			"[Unfinished step] is right here whenever you're ready.",
			"Ready to pick back up with [Unfinished step] on [topic]?",
			"Continuing with [Unfinished step] right where we left off.",
		},
		EvaluateScore: func(ctx UserJourneyContext) int {
			if ctx.UnfinishedStep != "" {
				return 90
			}
			return 10
		},
	})

	e.RegisterStyle(StyleDefinition{
		ID:             StyleAnticipationBuilder,
		Name:           "Anticipation builder",
		OptimalContext: "High-energy upcoming event or deadline approaching",
		Templates: []string{
			"Only [X days] until [event] — exciting stuff!",
			"The countdown is on: [X days] until [event].",
			"With [event] coming up in [X days], everything is taking shape.",
		},
		EvaluateScore: func(ctx UserJourneyContext) int {
			if ctx.Event != "" && ctx.DaysUntilEvent > 0 && ctx.DaysUntilEvent <= 14 {
				return 95
			}
			return 0
		},
	})

	e.RegisterStyle(StyleDefinition{
		ID:             StyleGentleNudge,
		Name:           "Gentle nudge",
		OptimalContext: "User has been absent for an extended duration",
		Templates: []string{
			"Been a little while — [topic] is still here for you.",
			"Welcome back — your [topic] workspace is right as you left it.",
			"It's been some time, but [topic] is ready whenever you are.",
		},
		EvaluateScore: func(ctx UserJourneyContext) int {
			if ctx.DaysSinceLastActive >= 5 && ctx.Topic != "" {
				return 95
			}
			return 10
		},
	})

	e.RegisterStyle(StyleDefinition{
		ID:             StyleCuriositySpark,
		Name:           "Curiosity spark",
		OptimalContext: "Unexplored angle or serendipitous idea available",
		Templates: []string{
			"Here's something you might not have considered for [topic]...",
			"An unexpected angle just surfaced for [topic].",
			"A fresh thought on [topic] to spark some new ideas.",
		},
		EvaluateScore: func(ctx UserJourneyContext) int {
			if ctx.CuriosityAngle != "" && ctx.Topic != "" {
				return 85
			}
			return 25
		},
	})

	e.RegisterStyle(StyleDefinition{
		ID:             StyleEmpatheticAck,
		Name:           "Empathetic acknowledgment",
		OptimalContext: "High stress, cognitive load, or complex problem",
		Templates: []string{
			"Navigating [specific challenge] takes patience — and you've got it.",
			"Tackling [specific challenge] is no small feat — let's take it step by step.",
			"Handling [specific challenge] can feel heavy, but steady progress is happening.",
		},
		EvaluateScore: func(ctx UserJourneyContext) int {
			if ctx.HighStress || ctx.SpecificChallenge != "" {
				return 95
			}
			return 0
		},
	})

	e.RegisterStyle(StyleDefinition{
		ID:             StyleLowPressureOpener,
		Name:           "Low-pressure opener",
		OptimalContext: "Casual, low-stakes hobby or simmering idea",
		Templates: []string{
			"Nothing urgent — just your [topic] notes, whenever.",
			"No rush at all — keeping your [topic] ideas warm.",
			"Whenever inspiration strikes, your [topic] plans are ready.",
		},
		EvaluateScore: func(ctx UserJourneyContext) int {
			if ctx.LowPressureHobby && ctx.Topic != "" {
				return 90
			}
			return 20
		},
	})
}

// RegisterStyle adds or updates a style definition.
func (e *Engine) RegisterStyle(def StyleDefinition) {
	if _, exists := e.styles[def.ID]; !exists {
		e.orderedIDs = append(e.orderedIDs, def.ID)
	}
	e.styles[def.ID] = def
}

// ValidateHeadline checks strict formatting and banned cliché criteria.
func ValidateHeadline(headline string) error {
	trimmed := strings.TrimSpace(headline)
	if !strings.HasPrefix(trimmed, "# ") {
		return ErrInvalidGreetingHeadline
	}

	clean := strings.TrimPrefix(trimmed, "# ")
	clean = strings.TrimSpace(clean)

	if len(clean) == 0 {
		return ErrInvalidGreetingHeadline
	}

	if len(clean) > 120 {
		return ErrHeadlineTooLong
	}

	// Must be a single sentence
	sentenceDelimiters := regexp.MustCompile(`[.?!]\s+[A-Z]`)
	if sentenceDelimiters.MatchString(clean) {
		return ErrInvalidGreetingHeadline
	}

	lower := strings.ToLower(clean)
	for _, cliche := range BannedCliches {
		if strings.HasPrefix(lower, cliche) || strings.Contains(lower, " "+cliche+" ") {
			return fmt.Errorf("%w: matched %q", ErrBannedCliche, cliche)
		}
	}

	if strings.Contains(clean, "[") && strings.Contains(clean, "]") {
		return ErrUnresolvedTemplateToken
	}

	return nil
}

// Generate selects the best style for the user journey context, suppressing recent styles and clichés.
func (e *Engine) Generate(ctx context.Context, uCtx UserJourneyContext, recentStyles []StyleID) (*GeneratedGreeting, error) {
	recentSet := make(map[StyleID]bool)
	for _, id := range recentStyles {
		recentSet[id] = true
	}

	bestScore := -1
	var candidateDefs []StyleDefinition

	for _, id := range e.orderedIDs {
		def := e.styles[id]
		if recentSet[id] {
			continue // Suppress recently used styles
		}
		score := 0
		if def.EvaluateScore != nil {
			score = def.EvaluateScore(uCtx)
		}
		if score > bestScore {
			bestScore = score
			candidateDefs = []StyleDefinition{def}
		} else if score == bestScore && score > 0 {
			candidateDefs = append(candidateDefs, def)
		}
	}

	if len(candidateDefs) == 0 {
		// Fallback: relax suppression
		for _, id := range e.orderedIDs {
			def := e.styles[id]
			score := 0
			if def.EvaluateScore != nil {
				score = def.EvaluateScore(uCtx)
			}
			if score > bestScore {
				bestScore = score
				candidateDefs = []StyleDefinition{def}
			} else if score == bestScore && score > 0 {
				candidateDefs = append(candidateDefs, def)
			}
		}
	}

	if len(candidateDefs) == 0 {
		return nil, ErrNoMatchingStyle
	}

	chosenDef := candidateDefs[e.rng.Intn(len(candidateDefs))]

	// Render templates
	for _, tmpl := range chosenDef.Templates {
		rendered := e.substituteVariables(tmpl, uCtx)
		headline := "# " + rendered
		if err := ValidateHeadline(headline); err == nil {
			return &GeneratedGreeting{
				StyleID:      chosenDef.ID,
				StyleName:    chosenDef.Name,
				Headline:     headline,
				CleanText:    rendered,
				TemplateUsed: tmpl,
			}, nil
		}
	}

	// Fallback to safe default if templates contained banned words or unresolved tokens
	fallbackText := fmt.Sprintf("Checking in on your progress with %s.", safeString(uCtx.Topic, "your project"))
	fallbackHeadline := "# " + fallbackText

	return &GeneratedGreeting{
		StyleID:      chosenDef.ID,
		StyleName:    chosenDef.Name,
		Headline:     fallbackHeadline,
		CleanText:    fallbackText,
		TemplateUsed: "fallback",
	}, nil
}

func (e *Engine) substituteVariables(tmpl string, uCtx UserJourneyContext) string {
	res := tmpl

	topic := safeString(uCtx.Topic, "your journey")
	res = strings.ReplaceAll(res, "[topic]", topic)

	decision := safeString(uCtx.Decision, "the decision")
	res = strings.ReplaceAll(res, "[decision]", decision)

	milestone := safeString(uCtx.Milestone, "the milestone")
	res = strings.ReplaceAll(res, "[milestone]", milestone)

	event := safeString(uCtx.Event, "the event")
	res = strings.ReplaceAll(res, "[event]", event)

	days := "a few days"
	if uCtx.DaysUntilEvent > 0 {
		days = fmt.Sprintf("%d days", uCtx.DaysUntilEvent)
	}
	res = strings.ReplaceAll(res, "[X days]", days)

	optionsStr := "your options"
	if len(uCtx.Options) >= 2 {
		optionsStr = fmt.Sprintf("%s vs %s", uCtx.Options[0], uCtx.Options[1])
	}
	res = strings.ReplaceAll(res, "[Option A vs B]", optionsStr)

	unfinished := safeString(uCtx.UnfinishedStep, "the next step")
	res = strings.ReplaceAll(res, "[Unfinished step]", unfinished)

	challenge := safeString(uCtx.SpecificChallenge, "this challenge")
	res = strings.ReplaceAll(res, "[specific challenge]", challenge)

	return res
}

func safeString(val, fallback string) string {
	val = strings.TrimSpace(val)
	if val == "" {
		return fallback
	}
	return val
}
