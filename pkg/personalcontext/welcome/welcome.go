package welcome

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Typed error hierarchy for the Welcome Prompt Suite and Dispatcher.
var (
	ErrNoDataSources          = errors.New("no connected data sources provided")
	ErrBannedClicheInWelcome  = errors.New("welcome prompt starts with a banned cliché opener")
	ErrInvasiveDataDetected   = errors.New("invasive medical or emotional data prohibited in welcome prompt")
	ErrMechanicalExposition   = errors.New("synthesizer response contains mechanical source exposition")
	ErrMissingTrifectaDomain  = errors.New("showcase of possibilities requires all three life domains (productivity, passion, curiosity)")
	ErrMissingChoiceCallToAction = errors.New("showcase of possibilities requires explicit choice call-to-action closing")
	ErrInvalidTransparencyQnA = errors.New("trustworthy guide requires all three mandatory privacy Q&A entries")
)

// Persona identifier constants.
type Persona string

const (
	PersonaProductivityPartner Persona = "z2vix5_pcontext_testing_helpful_productivity_partner"
	PersonaCreativeMuse        Persona = "creative_muse"
	PersonaTrustworthyGuide    Persona = "transparent_trustworthy_guide"
	PersonaInsightfulSynthesizer Persona = "z2vix5_pcontext_testing_insightful_synthesizer"
	PersonaShowcasePossibilities Persona = "z2vix5_pcontext_testing_showcase_of_possibilities"
)

// SourceType defines canonical personal context sources.
type SourceType string

const (
	SourceGmail         SourceType = "gmail"
	SourceCalendar      SourceType = "calendar"
	SourceTasks         SourceType = "tasks"
	SourcePhotos        SourceType = "photos"
	SourceSearchHistory SourceType = "search_history"
	SourceNotes         SourceType = "notes"
	SourcePermissions   SourceType = "permissions"
)

// DataItem represents an extracted personal context signal.
type DataItem struct {
	ID        string     `json:"id"`
	Source    SourceType `json:"source"`
	Timestamp time.Time  `json:"timestamp"`
	Category  string     `json:"category"` // e.g. "logistics", "passion", "curiosity", "privacy"
	Title     string     `json:"title"`
	Details   string     `json:"details"`
	Sensitive bool       `json:"sensitive"` // e.g. medical, psychiatric, or PII
}

// SynthesisRequest encapsulates inputs to the Welcome Prompt Suite.
type SynthesisRequest struct {
	Items         []DataItem `json:"items"`
	ForcedPersona Persona    `json:"forced_persona,omitempty"`
	UserInterest  string     `json:"user_interest,omitempty"`
}

// WelcomeResponse represents the synthesized welcome output.
type WelcomeResponse struct {
	Persona   Persona    `json:"persona"`
	Header    string     `json:"header"`
	Greeting  string     `json:"greeting"`
	Body      string     `json:"body"`
	Actions   []string   `json:"actions"`
	Sources   []SourceType `json:"sources"`
	Timestamp time.Time  `json:"timestamp"`
}

// Dispatcher coordinates persona selection and cross-source synthesis.
type Dispatcher struct{}

// NewDispatcher constructs a new Dispatcher.
func NewDispatcher() *Dispatcher {
	return &Dispatcher{}
}

// SelectPersona determines the most suitable persona given the available context signals.
func (d *Dispatcher) SelectPersona(items []DataItem) Persona {
	if len(items) == 0 {
		return PersonaTrustworthyGuide
	}

	sourceMap := make(map[SourceType]bool)
	categoryMap := make(map[string]bool)

	for _, item := range items {
		if !item.Sensitive {
			sourceMap[item.Source] = true
			if item.Category != "" {
				categoryMap[strings.ToLower(item.Category)] = true
			}
		}
	}

	// 1. If privacy or permission items dominate or only permissions source exists
	if sourceMap[SourcePermissions] && len(sourceMap) == 1 {
		return PersonaTrustworthyGuide
	}

	// 2. If all 3 domains are present -> Showcase of Possibilities
	if categoryMap["productivity"] && categoryMap["passion"] && categoryMap["curiosity"] {
		return PersonaShowcasePossibilities
	}

	// 3. If multi-source (>= 2 independent non-permission sources)
	nonPermCount := 0
	for s := range sourceMap {
		if s != SourcePermissions {
			nonPermCount++
		}
	}
	if nonPermCount >= 2 {
		return PersonaInsightfulSynthesizer
	}

	// 4. Creative sources
	if sourceMap[SourcePhotos] || sourceMap[SourceSearchHistory] || categoryMap["passion"] {
		return PersonaCreativeMuse
	}

	// 5. Default to Productivity Partner for logistics / tasks
	return PersonaProductivityPartner
}

// Synthesize generates the persona-driven welcome experience.
func (d *Dispatcher) Synthesize(ctx context.Context, req SynthesisRequest) (*WelcomeResponse, error) {
	// Filter sensitive items (medical, emotional, psychiatric)
	var safeItems []DataItem
	for _, item := range req.Items {
		if !item.Sensitive && !d.isInvasive(item.Details) {
			safeItems = append(safeItems, item)
		}
	}

	persona := req.ForcedPersona
	if persona == "" {
		persona = d.SelectPersona(safeItems)
	}

	var resp *WelcomeResponse
	var err error

	switch persona {
	case PersonaProductivityPartner:
		resp, err = d.synthesizeProductivityPartner(safeItems)
	case PersonaCreativeMuse:
		resp, err = d.synthesizeCreativeMuse(safeItems)
	case PersonaTrustworthyGuide:
		resp, err = d.synthesizeTrustworthyGuide(req.UserInterest)
	case PersonaInsightfulSynthesizer:
		resp, err = d.synthesizeInsightfulSynthesizer(safeItems)
	case PersonaShowcasePossibilities:
		resp, err = d.synthesizeShowcasePossibilities(safeItems)
	default:
		resp, err = d.synthesizeProductivityPartner(safeItems)
	}

	if err != nil {
		return nil, err
	}

	resp.Timestamp = time.Now()
	return resp, nil
}

func (d *Dispatcher) synthesizeProductivityPartner(items []DataItem) (*WelcomeResponse, error) {
	var logistics []string
	sources := make(map[SourceType]bool)

	for _, it := range items {
		sources[it.Source] = true
		// Goldilocks rule: Never state trivial facts like flight numbers. Connect to actionable value.
		cleanTitle := d.stripTrivialFlightNumbers(it.Title)
		logistics = append(logistics, cleanTitle)
		if len(logistics) >= 3 {
			break
		}
	}

	greeting := "Welcome! I'm here as your productivity partner."
	if len(logistics) == 0 {
		logistics = append(logistics, "your upcoming schedule and tasks")
	}

	body := fmt.Sprintf("I noticed %s on your horizon. I can help organize details, prepare agendas, or find nearby recommendations whenever you're ready.", strings.Join(logistics, ", "))

	var actions []string
	for _, l := range logistics {
		actions = append(actions, fmt.Sprintf("Draft a prep checklist for %s", l))
	}

	return &WelcomeResponse{
		Persona:  PersonaProductivityPartner,
		Header:   "# Productivity & Logistics Overview",
		Greeting: greeting,
		Body:     body,
		Actions:  actions,
		Sources:  d.sourceKeys(sources),
	}, nil
}

func (d *Dispatcher) synthesizeCreativeMuse(items []DataItem) (*WelcomeResponse, error) {
	var passion string
	sources := make(map[SourceType]bool)

	for _, it := range items {
		sources[it.Source] = true
		if it.Category == "passion" || it.Category == "hobby" || it.Category == "creative" {
			passion = it.Title
			break
		}
	}

	if passion == "" && len(items) > 0 {
		passion = items[0].Title
	}
	if passion == "" {
		passion = "your creative projects"
	}

	greeting := fmt.Sprintf("Excited to explore %s with you.", passion)
	body := fmt.Sprintf("Whether you are brainstorming new concepts, refining techniques, or gathering references, I'm here to support your creative journey.")

	return &WelcomeResponse{
		Persona:  PersonaCreativeMuse,
		Header:   "# Creative Exploration",
		Greeting: greeting,
		Body:     body,
		Actions: []string{
			fmt.Sprintf("Explore fresh ideas for %s", passion),
			"Brainstorm project directions",
		},
		Sources: d.sourceKeys(sources),
	}, nil
}

func (d *Dispatcher) synthesizeTrustworthyGuide(userInterest string) (*WelcomeResponse, error) {
	hook := "Because you connect your personal context, I can deliver proactive, personalized support across your active topics."
	if userInterest != "" {
		hook = fmt.Sprintf("Because you follow %s, I can tailor updates and research directly to your workflow.", userInterest)
	}

	qna := strings.Join([]string{
		"### Transparency & Trust Guide",
		"**Q: How will my data be used?**",
		"Only to answer your direct requests and contextualize your topics. Never for ads, and never sold or shared.",
		"",
		"**Q: How do I control what you know?**",
		"Manage or delete connected sources anytime in Personal Context settings, or simply tell me to forget a topic.",
		"",
		"**Q: How is my data protected?**",
		"All data is encrypted in transit and at rest with strict zero-cost local sandboxing.",
	}, "\n")

	body := fmt.Sprintf("%s\n\n%s", hook, qna)

	return &WelcomeResponse{
		Persona:  PersonaTrustworthyGuide,
		Header:   "# Radical Transparency & Privacy Guide",
		Greeting: "Welcome to your personal assistant.",
		Body:     body,
		Actions: []string{
			"Review data boundaries",
			"Configure privacy preferences",
		},
		Sources: []SourceType{SourcePermissions},
	}, nil
}

func (d *Dispatcher) synthesizeInsightfulSynthesizer(items []DataItem) (*WelcomeResponse, error) {
	sources := make(map[SourceType]bool)
	var topics []string

	for _, it := range items {
		sources[it.Source] = true
		topics = append(topics, it.Title)
	}

	// Must have >= 2 sources
	if len(sources) < 2 {
		return nil, fmt.Errorf("insightful synthesizer requires at least 2 distinct data sources, got %d", len(sources))
	}

	// Fluid insight without mechanical exposition ("I noticed you're preparing for X and Y" vs "I saw X in your email and Y in your calendar")
	greeting := "Looking across your recent notes and plans, a few exciting threads connect."
	body := fmt.Sprintf("I noticed you are focusing on %s. Connecting these threads can save you time and streamline your next steps.", strings.Join(topics, " and "))

	// Check mechanical exposition guardrail
	if strings.Contains(strings.ToLower(body), "i saw") || strings.Contains(strings.ToLower(body), "in your email") {
		return nil, ErrMechanicalExposition
	}

	return &WelcomeResponse{
		Persona:  PersonaInsightfulSynthesizer,
		Header:   "# Cross-Source Context Synthesis",
		Greeting: greeting,
		Body:     body,
		Actions: []string{
			"Connect these plans into a single checklist",
			"Synthesize shared timelines",
		},
		Sources: d.sourceKeys(sources),
	}, nil
}

func (d *Dispatcher) synthesizeShowcasePossibilities(items []DataItem) (*WelcomeResponse, error) {
	var prod, pass, cur DataItem
	var hasProd, hasPass, hasCur bool
	sources := make(map[SourceType]bool)

	for _, it := range items {
		sources[it.Source] = true
		cat := strings.ToLower(it.Category)
		if cat == "productivity" && !hasProd {
			prod = it
			hasProd = true
		} else if cat == "passion" && !hasPass {
			pass = it
			hasPass = true
		} else if cat == "curiosity" && !hasCur {
			cur = it
			hasCur = true
		}
	}

	if !hasProd || !hasPass || !hasCur {
		return nil, ErrMissingTrifectaDomain
	}

	greeting := "Welcome! Here is a snapshot of how I can assist across your week."
	body := strings.Join([]string{
		fmt.Sprintf("• **Productivity**: %s — ready for logistics and preparation.", prod.Title),
		fmt.Sprintf("• **Passion**: %s — explore creative ideas and inspiration.", pass.Title),
		fmt.Sprintf("• **Curiosity**: %s — dive into in-depth research and learning.", cur.Title),
		"",
		"Which of these areas would you like to explore first?",
	}, "\n")

	return &WelcomeResponse{
		Persona:  PersonaShowcasePossibilities,
		Header:   "# Showcase of Possibilities",
		Greeting: greeting,
		Body:     body,
		Actions: []string{
			fmt.Sprintf("Explore Productivity: %s", prod.Title),
			fmt.Sprintf("Explore Passion: %s", pass.Title),
			fmt.Sprintf("Explore Curiosity: %s", cur.Title),
		},
		Sources: d.sourceKeys(sources),
	}, nil
}

func (d *Dispatcher) isInvasive(details string) bool {
	lower := strings.ToLower(details)
	invasiveKeywords := []string{
		"medical diagnosis",
		"prescription",
		"psychotherapy",
		"confidential health",
		"blood test",
	}
	for _, kw := range invasiveKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func (d *Dispatcher) stripTrivialFlightNumbers(title string) string {
	re := regexp.MustCompile(`(?i)\b(flight|flt|ua|aa|dl|ba)\s*#?\d{2,4}\b`)
	cleaned := re.ReplaceAllString(title, "flight")
	return strings.TrimSpace(cleaned)
}

func (d *Dispatcher) sourceKeys(m map[SourceType]bool) []SourceType {
	var keys []SourceType
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
