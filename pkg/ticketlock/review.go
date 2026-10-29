package ticketlock

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"Dendrite/pkg/llmchat"
	"Dendrite/pkg/rag"
)

// Verdict statuses. A criterion is met only when the model copied a line from the change that really shows it.
const (
	StatusMet      = "met"
	StatusNotMet   = "not_met"
	StatusUnproven = "unproven"
)

const (
	// topHunks is how many of the most relevant hunks the model sees for one criterion.
	topHunks = 5
	// hunkBytes caps one hunk in the prompt.
	hunkBytes = 1500
	// minEvidence is the shortest evidence line that counts: a quoted "}" proves nothing.
	minEvidence = 12
)

// testCriterion matches a criterion that is about tests or coverage.
var testCriterion = regexp.MustCompile(`(?i)\b(tests?|tested|testing|rstest|cases?|coverage|assert\w*|fixtures?)\b`)

const proofPrompt = "You decide whether ONE acceptance criterion of a ticket is fulfilled by a code change. You are shown the change as a " +
	"summary and as excerpts of its most relevant parts. Answer with ONLY a JSON object: " +
	`{"verdict":"met","evidence":"one line copied exactly from the excerpts","reason":"one short sentence"}. ` +
	`verdict is "met" only if you can copy a line of the excerpts that shows it, "not_met" when the excerpts show the change does the opposite or leaves it out, ` +
	`"unclear" when the excerpts do not show it either way. The evidence line must appear in the excerpts word for word.`

const confirmPrompt = "You check ONE claim about code. You are given an acceptance criterion and a line of code (with its surroundings) that is " +
	`said to show it. Answer with ONLY a JSON object: {"shows":true,"why":"one short sentence"}. ` +
	"Answer true only if this code, by what it does, makes the criterion true. Code that merely mentions related words, belongs to something else, or only tests the feature does not show it: answer false."

// ProofReviewer judges acceptance criteria one at a time, each against the hunks of the change most relevant to
// it, and accepts "met" only with evidence it can find in what the model was shown.
type ProofReviewer struct {
	Chat llmchat.Completer
	Rank Ranker
}

// Review returns one verdict per criterion, in order. An error means the model could not be asked at all.
func (p ProofReviewer) Review(ctx context.Context, in ReviewInput) ([]ACVerdict, error) {
	hunks := splitHunks(in.Diff)
	out := make([]ACVerdict, len(in.Criteria))
	for i, criterion := range in.Criteria {
		out[i] = ACVerdict{AC: acID(acItem.FindString(criterion)), Status: StatusUnproven}
		if len(hunks) == 0 {
			out[i].Reason = "the ticket has no changes to review"
			continue
		}
		v, err := p.judge(ctx, in, criterion, hunks)
		if err != nil {
			return nil, err
		}
		v.AC = out[i].AC
		out[i] = v
	}
	return out, nil
}

// judge asks about one criterion: the model gets the summary and the best hunks, and its answer is read and checked.
func (p ProofReviewer) judge(ctx context.Context, in ReviewInput, criterion string, hunks []Hunk) (ACVerdict, error) {
	// Tests only show that something is tested, so they are evidence only for a criterion about tests.
	var docs []rag.RankDocument
	for i, h := range hunks {
		if !h.Test || testCriterion.MatchString(criterion) {
			docs = append(docs, rag.RankDocument{ID: strconv.Itoa(i), Text: h.Path + "\n" + capText(h.Text, hunkBytes)})
		}
	}
	if len(docs) == 0 { // a change made only of tests
		for i, h := range hunks {
			docs = append(docs, rag.RankDocument{ID: strconv.Itoa(i), Text: h.Path + "\n" + capText(h.Text, hunkBytes)})
		}
	}
	ids, err := p.Rank(ctx, criterion, docs, topHunks)
	if err != nil {
		return ACVerdict{}, err
	}
	var excerpts strings.Builder
	for _, id := range ids {
		if i, err := strconv.Atoi(id); err == nil && i >= 0 && i < len(hunks) {
			fmt.Fprintf(&excerpts, "=== %s ===\n%s\n", hunks[i].Path, capText(hunks[i].Text, hunkBytes))
		}
	}
	user := fmt.Sprintf("Ticket %s: %s\n\nCriterion: %s\n\nSummary of the change:\n%s\nExcerpts:\n%s",
		in.Ticket, capText(in.Description, 600), criterion, capText(changeSummary(hunks), 2500), excerpts.String())

	answer, err := p.Chat.Complete(ctx, proofPrompt, user)
	if err != nil {
		return ACVerdict{}, err
	}
	read, readErr := readProof(answer)
	if readErr != nil {
		// A chatty model often reads the reminder right: ask once more, quoting why the first answer was unusable.
		if answer, err = p.Chat.Complete(ctx, proofPrompt, user+"\n\nYour previous answer could not be read ("+readErr.Error()+"). Reply with ONLY the JSON object, nothing else."); err != nil {
			return ACVerdict{}, err
		}
		if read, readErr = readProof(answer); readErr != nil {
			return ACVerdict{Status: StatusUnproven, Reason: "the model's answer could not be read: " + readErr.Error()}, nil
		}
	}
	verdict := check(read, excerpts.String())
	if verdict.Status != StatusMet {
		return verdict, nil
	}
	return p.confirm(ctx, criterion, verdict, hunks, ids)
}

// confirm asks a second, narrow question about a "met": does the quoted line, in its hunk, really make the
// criterion true? A model that quoted something real but unrelated usually says no here.
func (p ProofReviewer) confirm(ctx context.Context, criterion string, verdict ACVerdict, hunks []Hunk, ids []string) (ACVerdict, error) {
	surroundings := ""
	for _, id := range ids {
		if i, err := strconv.Atoi(id); err == nil && i >= 0 && i < len(hunks) && evidenceFound(verdict.Evidence, hunks[i].Text) {
			surroundings = "=== " + hunks[i].Path + " ===\n" + capText(hunks[i].Text, hunkBytes)
			break
		}
	}
	user := fmt.Sprintf("Criterion: %s\n\nThe line said to show it:\n%s\n\nIts surroundings:\n%s", criterion, verdict.Evidence, surroundings)
	answer, err := p.Chat.Complete(ctx, confirmPrompt, user)
	if err != nil {
		return ACVerdict{}, err
	}
	raw, jsonErr := llmchat.JSON(answer, '{', '}')
	var c struct {
		Shows bool   `json:"shows"`
		Why   string `json:"why"`
	}
	if jsonErr != nil || json.Unmarshal([]byte(raw), &c) != nil || !c.Shows {
		reason := firstNonEmpty(c.Why, "the answer to the follow-up question could not be read")
		return ACVerdict{Status: StatusUnproven, Evidence: verdict.Evidence, Reason: "the quoted line was not confirmed as showing it: " + reason}, nil
	}
	return verdict, nil
}

// proof is the model's answer about one criterion.
type proof struct {
	Verdict  string `json:"verdict"`
	Evidence string `json:"evidence"`
	Reason   string `json:"reason"`
}

func readProof(answer string) (proof, error) {
	raw, err := llmchat.JSON(answer, '{', '}')
	if err != nil {
		return proof{}, err
	}
	var p proof
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return proof{}, fmt.Errorf("the answer is not the expected object: %w", err)
	}
	return p, nil
}

// check turns the model's answer into a verdict. "met" stands only when its evidence is in the excerpts.
func check(p proof, excerpts string) ACVerdict {
	switch strings.ToLower(strings.TrimSpace(p.Verdict)) {
	case "met":
		if !evidenceFound(p.Evidence, excerpts) {
			return ACVerdict{Status: StatusUnproven, Evidence: p.Evidence, Reason: "the model said met, but the line it quoted is not in the change it was shown: " + p.Reason}
		}
		return ACVerdict{Status: StatusMet, Evidence: p.Evidence, Reason: p.Reason}
	case "not_met":
		return ACVerdict{Status: StatusNotMet, Reason: p.Reason}
	default:
		return ACVerdict{Status: StatusUnproven, Reason: firstNonEmpty(p.Reason, "the excerpts do not show it either way")}
	}
}

// evidenceFound reports whether the quoted line appears in the excerpts, ignoring spacing and diff markers.
func evidenceFound(evidence, excerpts string) bool {
	quote := normalizeDiffText(evidence)
	return len([]rune(quote)) >= minEvidence && strings.Contains(normalizeDiffText(excerpts), quote)
}

// normalizeDiffText drops the +/- marker that opens a diff line and collapses all whitespace.
func normalizeDiffText(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimLeft(l, "+- \t")
	}
	return strings.Join(strings.Fields(strings.Join(lines, " ")), " ")
}

// capText keeps the first n bytes of s and says when it cut.
func capText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n[cut]\n"
}
