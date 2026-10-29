package personalcontext

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Typed error hierarchy for Zero State validation and state machine operations.
var (
	ErrBannedCliche       = errors.New("greeting starts with a banned cliché opener")
	ErrInvalidGreeting    = errors.New("greeting must be a single-sentence headline starting with '# '")
	ErrInvalidSummary     = errors.New("summary must be between 1 and 2 sentences")
	ErrTooManyNextSteps   = errors.New("suggested next steps must contain at most 2 bullets")
	ErrNextStepTooLong    = errors.New("suggested next step exceeds maximum length of 75 characters")
	ErrNextStepActionVerb = errors.New("suggested next step must start with an action verb")
	ErrMissingTopicMemory = errors.New("missing structured Topic Memory code block")
	ErrInvalidSpacing     = errors.New("must contain exactly 5 empty spacing lines before Topic Memory")
	ErrTurnViolation      = errors.New("turn 2+ response must not contain Zero State scaffolding components")
)

// BannedCliches lists the strictly prohibited greeting openers.
var BannedCliches = []string{
	"ready to",
	"let's",
	"lets",
	"time to",
	"looking to",
	"want to",
	"how about",
}

// CanonicalActionVerbs lists common allowed action verbs for next step suggestions.
var CanonicalActionVerbs = map[string]bool{
	"review":       true,
	"compare":      true,
	"draft":        true,
	"walk through": true,
	"prepare":      true,
	"find":         true,
	"identify":     true,
	"explore":      true,
	"outline":      true,
	"compile":      true,
	"check":        true,
	"schedule":     true,
	"plan":         true,
	"summarize":    true,
	"inspect":      true,
	"organize":     true,
}

// GreetingStyle represents one of the 10 canonical dynamic greeting styles.
type GreetingStyle string

const (
	StyleThoughtfulCheckin    GreetingStyle = "G-01" // Thoughtful check-in
	StyleProgressCelebration  GreetingStyle = "G-02" // Progress celebration
	StyleReflectionPrompt     GreetingStyle = "G-03" // Reflection prompt
	StyleCallbackToDetail     GreetingStyle = "G-04" // Callback to specific detail
	StyleInvitationToContinue GreetingStyle = "G-05" // Invitation to continue
	StyleAnticipationBuilder  GreetingStyle = "G-06" // Anticipation builder
	StyleGentleNudge          GreetingStyle = "G-07" // Gentle nudge
	StyleMilestoneRecap       GreetingStyle = "G-08" // Milestone recap
	StyleStrategicPause       GreetingStyle = "G-09" // Strategic pause
	StyleCreativeSpark        GreetingStyle = "G-10" // Creative spark
)

// MemorySection represents a formatted section within the Topic Memory block.
type MemorySection struct {
	Emoji string   `json:"emoji"`
	Title string   `json:"title"`
	Facts []string `json:"facts"`
}

// ZeroStateRequest encapsulates the inputs needed to synthesize a Topic Zero State.
type ZeroStateRequest struct {
	Topic            string          `json:"topic"`
	Greeting         string          `json:"greeting"`
	GreetingStyle    GreetingStyle   `json:"greeting_style"`
	Summary          string          `json:"summary"`
	NextSteps        []string        `json:"next_steps"`
	MemorySections   []MemorySection `json:"memory_sections"`
	RawMemory        string          `json:"raw_memory"`
	AmbiguousContext bool            `json:"ambiguous_context"`
}

// ZeroStateEngine handles synthesis and deterministic validation of Topic Zero States.
type ZeroStateEngine struct{}

// NewZeroStateEngine constructs a new engine.
func NewZeroStateEngine() *ZeroStateEngine {
	return &ZeroStateEngine{}
}

// Render synthesizes the five-part Zero State markdown document.
func (e *ZeroStateEngine) Render(req ZeroStateRequest) (string, error) {
	greeting := strings.TrimSpace(req.Greeting)
	if greeting == "" {
		greeting = e.defaultGreeting(req.Topic, req.GreetingStyle)
	}

	if err := e.validateGreeting(greeting); err != nil {
		return "", err
	}

	summary := strings.TrimSpace(req.Summary)
	if summary == "" {
		summary = fmt.Sprintf("Here is the latest progress on %s.", req.Topic)
	}

	var sb strings.Builder
	sb.WriteString("# ")
	sb.WriteString(greeting)
	sb.WriteString("\n\n")
	sb.WriteString(summary)
	sb.WriteString("\n")

	// Component 3: Suggested Next Steps (Optional, omitted if ambiguous or empty)
	if !req.AmbiguousContext && len(req.NextSteps) > 0 {
		if len(req.NextSteps) > 2 {
			return "", ErrTooManyNextSteps
		}

		sb.WriteString("\n## Suggested next steps, in bullet points\n\n")
		for _, step := range req.NextSteps {
			stepClean := strings.TrimSpace(step)
			if utf8.RuneCountInString(stepClean) > 75 {
				return "", fmt.Errorf("%w: '%s' has %d chars (max 75)", ErrNextStepTooLong, stepClean, utf8.RuneCountInString(stepClean))
			}
			if !e.hasActionVerbPrefix(stepClean) {
				return "", fmt.Errorf("%w: '%s'", ErrNextStepActionVerb, stepClean)
			}
			sb.WriteString("- ")
			sb.WriteString(stepClean)
			sb.WriteString("\n")
		}
	}

	// Component 4: Exactly 5 empty rows
	sb.WriteString("\n\n\n\n\n")

	// Component 5: Topic Memory Code Block
	sb.WriteString("```\nTopic memory\n\n")
	sb.WriteString("All the content below won't be shown on the Topics Zero State.\n")
	sb.WriteString("They will be used in responses, and will be 1 tap away if the user\n")
	sb.WriteString("wants to take a deeper look at the Topic's memory.\n\n")

	if len(req.MemorySections) > 0 {
		for _, sec := range req.MemorySections {
			if sec.Emoji != "" {
				sb.WriteString(fmt.Sprintf("%s %s\n", sec.Emoji, sec.Title))
			} else {
				sb.WriteString(fmt.Sprintf("• %s\n", sec.Title))
			}
			for _, fact := range sec.Facts {
				sb.WriteString(fmt.Sprintf("  - %s\n", fact))
			}
			sb.WriteString("\n")
		}
	} else if strings.TrimSpace(req.RawMemory) != "" {
		sb.WriteString(strings.TrimSpace(req.RawMemory))
		sb.WriteString("\n")
	} else {
		sb.WriteString(fmt.Sprintf("📌 Initial context for %s.\n", req.Topic))
	}
	sb.WriteString("```")

	return sb.String(), nil
}

// Validate verifies whether an existing Zero State string conforms to the FEAT-101 specification.
func (e *ZeroStateEngine) Validate(output string) error {
	lines := strings.Split(output, "\n")
	if len(lines) < 7 {
		return errors.New("output too short to be a valid Zero State")
	}

	// 1. Must start with '# '
	if !strings.HasPrefix(lines[0], "# ") {
		return ErrInvalidGreeting
	}
	greetingText := strings.TrimPrefix(lines[0], "# ")
	if err := e.validateGreeting(greetingText); err != nil {
		return err
	}

	// 2. Must contain Topic memory code block
	if !strings.Contains(output, "```\nTopic memory") || !strings.HasSuffix(strings.TrimSpace(output), "```") {
		return ErrMissingTopicMemory
	}

	// 3. Spacing validation: must contain 5 consecutive empty rows before the code block
	if !strings.Contains(output, "\n\n\n\n\n```") {
		return ErrInvalidSpacing
	}

	// 4. Validate next steps if present
	if strings.Contains(output, "## Suggested next steps, in bullet points") {
		idx := strings.Index(output, "## Suggested next steps, in bullet points")
		endIdx := strings.Index(output[idx:], "\n\n\n\n\n```")
		if endIdx == -1 {
			return ErrInvalidSpacing
		}
		sub := output[idx : idx+endIdx]
		bulletCount := 0
		for _, line := range strings.Split(sub, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
				bulletCount++
				stepText := strings.TrimSpace(trimmed[2:])
				if utf8.RuneCountInString(stepText) > 75 {
					return ErrNextStepTooLong
				}
				if !e.hasActionVerbPrefix(stepText) {
					return ErrNextStepActionVerb
				}
			}
		}
		if bulletCount > 2 {
			return ErrTooManyNextSteps
		}
	}

	return nil
}

func (e *ZeroStateEngine) validateGreeting(greeting string) error {
	lower := strings.ToLower(strings.TrimSpace(greeting))
	for _, cliche := range BannedCliches {
		if strings.HasPrefix(lower, cliche) {
			return fmt.Errorf("%w: starts with '%s'", ErrBannedCliche, cliche)
		}
	}
	return nil
}

func (e *ZeroStateEngine) hasActionVerbPrefix(step string) bool {
	lower := strings.ToLower(strings.TrimSpace(step))
	words := strings.Fields(lower)
	if len(words) == 0 {
		return false
	}

	// Check 2-word action verbs (e.g., "walk through")
	if len(words) >= 2 {
		twoWord := words[0] + " " + words[1]
		if CanonicalActionVerbs[twoWord] {
			return true
		}
	}

	// Check 1-word action verbs
	return CanonicalActionVerbs[words[0]]
}

func (e *ZeroStateEngine) defaultGreeting(topic string, style GreetingStyle) string {
	switch style {
	case StyleProgressCelebration:
		return fmt.Sprintf("Major progress on %s \u2014 here is where we stand.", topic)
	case StyleReflectionPrompt:
		return fmt.Sprintf("A lot has happened with %s \u2014 worth a quick look back?", topic)
	case StyleCallbackToDetail:
		return fmt.Sprintf("Revisiting the key decisions for %s.", topic)
	case StyleInvitationToContinue:
		return fmt.Sprintf("%s is set up and ready to continue whenever you'd like.", topic)
	case StyleAnticipationBuilder:
		return fmt.Sprintf("Approaching the next major milestone for %s.", topic)
	case StyleGentleNudge:
		return fmt.Sprintf("Picking up right where we left off with %s.", topic)
	case StyleMilestoneRecap:
		return fmt.Sprintf("Recapping recent achievements on %s.", topic)
	case StyleStrategicPause:
		return fmt.Sprintf("Taking a step back to evaluate the path forward for %s.", topic)
	case StyleCreativeSpark:
		return fmt.Sprintf("Fresh ideas and directions for %s.", topic)
	case StyleThoughtfulCheckin:
		fallthrough
	default:
		return fmt.Sprintf("How is %s coming along?", topic)
	}
}

// TurnState represents the lifecycle state of a topic session.
type TurnState string

const (
	Turn1ZeroState  TurnState = "Turn1_ZeroState"
	TurnNNormalChat TurnState = "TurnN_NormalChat"
	StateCorrection TurnState = "State_Correction"
)

// TurnInput represents the input message for a conversational turn.
type TurnInput struct {
	UserMessage   string            `json:"user_message"`
	UpdatedMemory *ZeroStateRequest `json:"updated_memory,omitempty"`
}

// TurnOutput represents the engine's response for a turn.
type TurnOutput struct {
	Turn           int       `json:"turn"`
	State          TurnState `json:"state"`
	Content        string    `json:"content"`
	IsZeroState    bool      `json:"is_zero_state"`
	Acknowledgment string    `json:"acknowledgment,omitempty"`
}

// MultiTurnStateMachine manages the turn-by-turn lifecycle state transitions.
type MultiTurnStateMachine struct {
	engine     *ZeroStateEngine
	currentReq ZeroStateRequest
	turnCount  int
	state      TurnState
}

// NewMultiTurnStateMachine initializes the state machine at Turn 0 before initiation.
func NewMultiTurnStateMachine(engine *ZeroStateEngine, initialReq ZeroStateRequest) *MultiTurnStateMachine {
	if engine == nil {
		engine = NewZeroStateEngine()
	}
	return &MultiTurnStateMachine{
		engine:     engine,
		currentReq: initialReq,
		turnCount:  0,
		state:      Turn1ZeroState,
	}
}

// CurrentTurn returns the 1-based turn number.
func (sm *MultiTurnStateMachine) CurrentTurn() int {
	return sm.turnCount
}

// CurrentState returns the active turn state.
func (sm *MultiTurnStateMachine) CurrentState() TurnState {
	return sm.state
}

// Reset resets the state machine back to Turn 0.
func (sm *MultiTurnStateMachine) Reset(newReq *ZeroStateRequest) {
	sm.turnCount = 0
	sm.state = Turn1ZeroState
	if newReq != nil {
		sm.currentReq = *newReq
	}
}

// ProcessTurn executes the state machine logic for an incoming user interaction.
func (sm *MultiTurnStateMachine) ProcessTurn(ctx context.Context, input TurnInput) (*TurnOutput, error) {
	sm.turnCount++

	// Turn 1: Always generates Zero State
	if sm.turnCount == 1 {
		sm.state = Turn1ZeroState
		rendered, err := sm.engine.Render(sm.currentReq)
		if err != nil {
			return nil, err
		}
		return &TurnOutput{
			Turn:        1,
			State:       Turn1ZeroState,
			Content:     rendered,
			IsZeroState: true,
		}, nil
	}

	// Turn 2+: Check for explicit correction / scope change
	if sm.IsCorrectionTrigger(input.UserMessage) {
		sm.state = StateCorrection
		if input.UpdatedMemory != nil {
			sm.currentReq = *input.UpdatedMemory
		}

		ack := "Got it, I've updated the topic memory to reflect that."
		rendered, err := sm.engine.Render(sm.currentReq)
		if err != nil {
			return nil, err
		}

		fullContent := fmt.Sprintf("%s\n\n%s", ack, rendered)
		return &TurnOutput{
			Turn:           sm.turnCount,
			State:          StateCorrection,
			Content:        fullContent,
			IsZeroState:    true,
			Acknowledgment: ack,
		}, nil
	}

	// Normal conversational turn
	sm.state = TurnNNormalChat
	reply := fmt.Sprintf("Understood. I am continuing to support your work on %s.", sm.currentReq.Topic)

	// Validate strict negative constraints for Turn 2+
	if err := sm.validateNormalChat(reply); err != nil {
		return nil, err
	}

	return &TurnOutput{
		Turn:        sm.turnCount,
		State:       TurnNNormalChat,
		Content:     reply,
		IsZeroState: false,
	}, nil
}

// IsCorrectionTrigger evaluates whether the user's message commands a memory correction.
func (sm *MultiTurnStateMachine) IsCorrectionTrigger(msg string) bool {
	lower := strings.ToLower(msg)
	triggers := []string{
		"actually,",
		"actually ",
		"update my",
		"update the",
		"change my",
		"change the",
		"correction:",
		"please update",
		"i canceled",
		"i cancelled",
		"i decided to change",
		"scratch that",
	}

	for _, t := range triggers {
		if strings.Contains(lower, t) {
			return true
		}
	}
	return false
}

// validateNormalChat verifies that Turn 2+ does not regurgitate Turn 1 Zero State scaffolding.
func (sm *MultiTurnStateMachine) validateNormalChat(content string) error {
	reGreeting := regexp.MustCompile(`(?m)^#\s+`)
	if reGreeting.MatchString(content) {
		return fmt.Errorf("%w: contains '# ' greeting headline", ErrTurnViolation)
	}
	if strings.Contains(content, "## Suggested next steps, in bullet points") {
		return fmt.Errorf("%w: contains suggested next steps block", ErrTurnViolation)
	}
	if strings.Contains(content, "Topic memory") {
		return fmt.Errorf("%w: contains Topic memory code block", ErrTurnViolation)
	}
	return nil
}
