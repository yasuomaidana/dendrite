package memory

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Typed error hierarchy for Topic Memory parser and Fidelity Guard.
var (
	ErrMalformedBlock         = errors.New("malformed topic memory code block")
	ErrMissingDisclaimer      = errors.New("missing or altered topic memory header disclaimer")
	ErrNoSectionsFound        = errors.New("topic memory must contain at least one section")
	ErrMissingEmojiHeader     = errors.New("each topic memory section must start with an emoji icon")
	ErrExtrapolationDetected  = errors.New("zero-extrapolation violation: unsupported fact or hallucinated entity detected")
	ErrInformationLossDetected = errors.New("information loss violation: key entities from original context missing")
)

// Standard mandatory disclaimer header.
const MandatoryDisclaimer = "Topic memory\n\nAll the content below won't be shown on the Topics Zero State.\nThey will be used in responses, and will be 1 tap away if the user\nwants to take a deeper look at the Topic's memory."

// Section represents an emoji-headed structured fact category.
type Section struct {
	Emoji string   `json:"emoji"`
	Title string   `json:"title"`
	Facts []string `json:"facts"`
}

// MemoryBlock represents the parsed lossless contents of a Topic Memory block.
type MemoryBlock struct {
	Disclaimer string    `json:"disclaimer"`
	Sections   []Section `json:"sections"`
}

// Parser handles bidirectional parsing and formatted serialization of Topic Memory blocks.
type Parser struct{}

// NewParser constructs a new Parser.
func NewParser() *Parser {
	return &Parser{}
}

// Parse extracts structured sections and facts from a fenced Topic Memory block.
func (p *Parser) Parse(raw string) (*MemoryBlock, error) {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "```") || !strings.HasSuffix(trimmed, "```") {
		return nil, ErrMalformedBlock
	}

	// Strip outer backticks
	content := trimmed[3 : len(trimmed)-3]
	content = strings.TrimSpace(content)

	// Check disclaimer
	if !strings.HasPrefix(content, "Topic memory") {
		return nil, ErrMissingDisclaimer
	}

	normContent := strings.ReplaceAll(content, "\r\n", "\n")
	normDisclaimer := strings.ReplaceAll(MandatoryDisclaimer, "\r\n", "\n")

	discIdx := strings.Index(normContent, normDisclaimer)
	if discIdx == -1 {
		return nil, ErrMissingDisclaimer
	}

	body := normContent[discIdx+len(normDisclaimer):]
	lines := strings.Split(body, "\n")

	var sections []Section
	var currentSec *Section

	for _, line := range lines {
		l := strings.TrimSpace(line)
		if l == "" {
			continue
		}

		// Check if line is a bulleted fact
		if strings.HasPrefix(l, "- ") || strings.HasPrefix(l, "* ") {
			if currentSec == nil {
				return nil, fmt.Errorf("%w: fact found before section header: %s", ErrMalformedBlock, l)
			}
			factText := strings.TrimSpace(l[2:])
			currentSec.Facts = append(currentSec.Facts, factText)
			continue
		}

		// Check if line is an indented continuation fact without bullet
		if strings.HasPrefix(line, "  ") && currentSec != nil {
			trimmedFact := strings.TrimSpace(line)
			currentSec.Facts = append(currentSec.Facts, trimmedFact)
			continue
		}

		// Section header line: starts with emoji or icon
		r, size := utf8.DecodeRuneInString(l)

		if isEmojiOrSymbol(r) {
			emojiBytes := size
			rem := l[size:]
			for len(rem) > 0 {
				nextR, nextSize := utf8.DecodeRuneInString(rem)
				if nextR == '\uFE0F' || nextR == '\uFE0E' || nextR == '\u200D' || isEmojiOrSymbol(nextR) {
					emojiBytes += nextSize
					rem = rem[nextSize:]
				} else {
					break
				}
			}

			emojiStr := l[:emojiBytes]
			secTitle := strings.TrimSpace(l[emojiBytes:])

			// If current section exists, commit it
			if currentSec != nil {
				sections = append(sections, *currentSec)
			}

			currentSec = &Section{
				Emoji: emojiStr,
				Title: secTitle,
				Facts: make([]string, 0),
			}
		} else {
			return nil, fmt.Errorf("%w: '%s' does not start with an emoji", ErrMissingEmojiHeader, l)
		}
	}

	if currentSec != nil {
		sections = append(sections, *currentSec)
	}

	if len(sections) == 0 {
		return nil, ErrNoSectionsFound
	}

	return &MemoryBlock{
		Disclaimer: MandatoryDisclaimer,
		Sections:   sections,
	}, nil
}

// Format serializes a MemoryBlock into a canonical fenced markdown block with word wrapping.
func (p *Parser) Format(mb *MemoryBlock, maxLineWidth int) string {
	if maxLineWidth <= 0 {
		maxLineWidth = 80
	}

	var sb strings.Builder
	sb.WriteString("```\n")
	sb.WriteString(MandatoryDisclaimer)
	sb.WriteString("\n\n")

	for i, sec := range mb.Sections {
		sb.WriteString(fmt.Sprintf("%s %s\n", sec.Emoji, sec.Title))
		for _, fact := range sec.Facts {
			wrapped := p.wrapFact(fact, maxLineWidth-4) // account for "  - " prefix
			for j, line := range wrapped {
				if j == 0 {
					sb.WriteString(fmt.Sprintf("  - %s\n", line))
				} else {
					sb.WriteString(fmt.Sprintf("    %s\n", line))
				}
			}
		}
		if i < len(mb.Sections)-1 {
			sb.WriteString("\n")
		}
	}

	sb.WriteString("```")
	return sb.String()
}

func (p *Parser) wrapFact(fact string, maxLen int) []string {
	if len(fact) <= maxLen {
		return []string{fact}
	}

	words := strings.Fields(fact)
	var lines []string
	var currentLine string

	for _, w := range words {
		if currentLine == "" {
			currentLine = w
		} else if len(currentLine)+1+len(w) <= maxLen {
			currentLine += " " + w
		} else {
			lines = append(lines, currentLine)
			currentLine = w
		}
	}
	if currentLine != "" {
		lines = append(lines, currentLine)
	}
	return lines
}

func isEmojiOrSymbol(r rune) bool {
	return unicode.IsSymbol(r) || unicode.Is(unicode.So, r) || (r >= 0x1F300 && r <= 0x1FAFF) || (r >= 0x2600 && r <= 0x27BF)
}

// FidelityGuardOptions provides parameters for the zero-extrapolation fidelity checker.
type FidelityGuardOptions struct {
	AllowFormattingModifications bool    `json:"allow_formatting_modifications"`
	MinFidelityRatio             float64 `json:"min_fidelity_ratio"` // default 1.0 (zero extrapolation)
}

// FidelityReport details the ground truth alignment.
type FidelityReport struct {
	OriginalTokens     int      `json:"original_tokens"`
	MemoryTokens       int      `json:"memory_tokens"`
	ExtrapolatedTerms  []string `json:"extrapolated_terms"`
	MissingKeyEntities []string `json:"missing_key_entities"`
	FidelityRatio      float64  `json:"fidelity_ratio"`
	Passed             bool     `json:"passed"`
}

// FidelityGuard verifies zero extrapolation and lossless preservation of user facts.
type FidelityGuard struct{}

// NewFidelityGuard constructs a new FidelityGuard.
func NewFidelityGuard() *FidelityGuard {
	return &FidelityGuard{}
}

// Verify ensures that every substantive term in the topic memory is strictly grounded in the original context.
func (g *FidelityGuard) Verify(originalContext string, mb *MemoryBlock, opts FidelityGuardOptions) (*FidelityReport, error) {
	if mb == nil || len(mb.Sections) == 0 {
		return nil, ErrNoSectionsFound
	}

	origNormalized := strings.ToLower(originalContext)
	origTokens := tokenizeSubstantive(originalContext)
	origTokenSet := make(map[string]bool)
	for _, t := range origTokens {
		origTokenSet[t] = true
	}

	var memoryText strings.Builder
	for _, sec := range mb.Sections {
		for _, fact := range sec.Facts {
			memoryText.WriteString(fact)
			memoryText.WriteString(" ")
		}
	}

	memTokens := tokenizeSubstantive(memoryText.String())
	var extrapolated []string
	groundedCount := 0

	for _, term := range memTokens {
		if origTokenSet[term] || strings.Contains(origNormalized, term) {
			groundedCount++
		} else {
			extrapolated = append(extrapolated, term)
		}
	}

	// Check missing key entities (numbers, uppercase entities from original context)
	keyEntities := extractKeyEntities(originalContext)
	var missing []string
	memLower := strings.ToLower(memoryText.String())
	for _, ent := range keyEntities {
		if !strings.Contains(memLower, strings.ToLower(ent)) {
			missing = append(missing, ent)
		}
	}

	totalMemTokens := len(memTokens)
	fidelityRatio := 1.0
	if totalMemTokens > 0 {
		fidelityRatio = float64(groundedCount) / float64(totalMemTokens)
	}

	threshold := opts.MinFidelityRatio
	if threshold <= 0 {
		threshold = 1.0
	}

	passed := len(extrapolated) == 0 && len(missing) == 0 && fidelityRatio >= threshold

	rep := &FidelityReport{
		OriginalTokens:     len(origTokens),
		MemoryTokens:       totalMemTokens,
		ExtrapolatedTerms:  extrapolated,
		MissingKeyEntities: missing,
		FidelityRatio:      fidelityRatio,
		Passed:             passed,
	}

	if len(missing) > 0 {
		return rep, fmt.Errorf("%w: %v", ErrInformationLossDetected, missing)
	}

	if len(extrapolated) > 0 || fidelityRatio < threshold {
		return rep, fmt.Errorf("%w: %v", ErrExtrapolationDetected, extrapolated)
	}

	return rep, nil
}

// Stop words to exclude from extrapolation checks (prepositions, conjunctions, structural words).
var stopWords = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "as": true, "at": true,
	"be": true, "but": true, "by": true, "for": true, "if": true, "in": true,
	"into": true, "is": true, "it": true, "no": true, "not": true, "of": true,
	"on": true, "or": true, "such": true, "that": true, "the": true, "their": true,
	"then": true, "there": true, "these": true, "they": true, "this": true, "to": true,
	"was": true, "will": true, "with": true, "under": true, "near": true, "from": true,
	"has": true, "have": true, "had": true, "all": true, "been": true, "per": true,
}

func tokenizeSubstantive(text string) []string {
	words := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})

	var res []string
	for _, w := range words {
		lower := strings.ToLower(w)
		if len(lower) > 1 && !stopWords[lower] {
			res = append(res, lower)
		}
	}
	return res
}

func extractKeyEntities(text string) []string {
	// Extract numbers, prices ($200), codes, dates
	reNumbers := regexp.MustCompile(`\$?\d+[\w/.-]*`)
	matches := reNumbers.FindAllString(text, -1)

	var entities []string
	seen := make(map[string]bool)
	for _, m := range matches {
		clean := strings.TrimRight(m, ".,;:!?")
		if clean != "" && !seen[clean] {
			seen[clean] = true
			entities = append(entities, clean)
		}
	}
	return entities
}
