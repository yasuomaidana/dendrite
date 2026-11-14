package plane

import (
	"bufio"
	"context"
	"regexp"
	"strings"

	"Dendrite/pkg/tracker"
)

var (
	devTicketLineRegex      = regexp.MustCompile(`^-\s+\[([ xX])\]\s+\*\*([A-Za-z0-9-]+)\*\*:\s+(.*?)(?:\s+\(([^)]+)\))?(?:\s+(?:\[|\*\()(?:State:\s*)?([A-Za-z0-9_ ]+?)(?:[|,]?\s*Priority:\s*([A-Za-z0-9_ ]+?))?(?:\]|\)\*))?$`)
	milestoneHdrRegex       = regexp.MustCompile(`^###\s+(Milestone\s+\d+:.*?)(?:\s+\(([^)]+)\))?$`)
	ticketIdPrefixRegex     = regexp.MustCompile(`\[?([A-Za-z0-9]+-\d+(?:-[A-Za-z]+-\d+)?)\]?`)
	complexityTableRowRegex = regexp.MustCompile(`\|\s*(?:\*\*\d+\*\*|\d+)\s*\|\s*\*?\*?([A-Za-z0-9-]+)\*?\*?\s*\|\s*([^|]+)\|\s*([^|]+)\|\s*(\d+)\s*SP\s*\|\s*([0-9.]+)\s*/\s*10`)
)

// ParsedDevTicket encapsulates all extracted metadata for a ticket from DEV_STATUS.md.
type ticketItem struct {
	id         string
	num        int
	title      string
	status     tracker.Status
	desc       string
	stateName  string
	sequenceID int
	targetPkg  string
	priority   string
	milestone  string
	project    string
}

type ParsedDevTicket struct {
	ID                 string
	Number             int
	Title              string
	TargetPackage      string
	Milestone          string
	FeatureID          string
	Status             tracker.Status
	StoryPoints        string
	ComplexityScore    string
	Goal               string
	GoverningSpecs     string
	AcceptanceCriteria []string
}

// DevStatusData encapsulates structured content parsed from DEV_STATUS.md.
type DevStatusData struct {
	ActiveMilestone string
	CurrentFocus    string
	LastUpdated     string
	Summary         string
	Tickets         []ParsedDevTicket
	Milestones      []string
}

// ParseDevStatus parses raw DEV_STATUS.md text into structured records.
func ParseDevStatus(content string) (*DevStatusData, error) {
	data := &DevStatusData{
		Tickets:    make([]ParsedDevTicket, 0),
		Milestones: make([]string, 0),
	}

	tableSpecs := make(map[string]struct {
		featID     string
		sp         string
		complexity string
	})

	scanner := bufio.NewScanner(strings.NewReader(content))
	currentMilestone := "General"
	seenMilestones := make(map[string]bool)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "- **Active Milestone**:") {
			data.ActiveMilestone = strings.TrimSpace(strings.TrimPrefix(line, "- **Active Milestone**:"))
			continue
		}
		if strings.HasPrefix(line, "- **Current Focus**:") {
			data.CurrentFocus = strings.TrimSpace(strings.TrimPrefix(line, "- **Current Focus**:"))
			continue
		}
		if strings.HasPrefix(line, "- **Last Updated**:") {
			data.LastUpdated = strings.TrimSpace(strings.TrimPrefix(line, "- **Last Updated**:"))
			continue
		}
		if strings.HasPrefix(line, "- **Summary**:") {
			data.Summary = strings.TrimSpace(strings.TrimPrefix(line, "- **Summary**:"))
			continue
		}

		if tblMatches := complexityTableRowRegex.FindStringSubmatch(line); len(tblMatches) > 5 {
			tID := tblMatches[1]
			featID := strings.TrimSpace(tblMatches[3])
			spStr := strings.TrimSpace(tblMatches[4])
			scoreStr := strings.TrimSpace(tblMatches[5])
			tableSpecs[tID] = struct {
				featID     string
				sp         string
				complexity string
			}{featID, spStr, scoreStr}
			continue
		}

		if mMatches := milestoneHdrRegex.FindStringSubmatch(line); len(mMatches) > 1 {
			currentMilestone = strings.TrimSpace(mMatches[1])
			if !seenMilestones[currentMilestone] {
				seenMilestones[currentMilestone] = true
				data.Milestones = append(data.Milestones, currentMilestone)
			}
			continue
		}

		if tMatches := devTicketLineRegex.FindStringSubmatch(line); len(tMatches) > 3 {
			isDone := strings.ToLower(tMatches[1]) == "x"
			id := tMatches[2]
			title := tMatches[3]
			pkg := ""
			if len(tMatches) > 4 && tMatches[4] != "" {
				pkg = strings.Trim(strings.TrimSpace(tMatches[4]), "`")
			}
			customStatus := ""
			if len(tMatches) > 5 && tMatches[5] != "" {
				customStatus = strings.TrimSpace(tMatches[5])
			}

			// "[State: To Verify, Priority: High]" -> "To Verify"
			stateToken, _, _ := strings.Cut(customStatus, ",")
			stateToken = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(stateToken), "State:"))
			st := tracker.StatusTodo
			if parsed, err := tracker.ParseStatus(stateToken); err == nil {
				st = parsed
			} else if strings.Contains(data.CurrentFocus, id) {
				st = tracker.StatusInProgress
			}
			if isDone {
				st = tracker.StatusDone
			}

			num := extractTicketNumber(id)

			ticket := ParsedDevTicket{
				ID:            id,
				Number:        num,
				Title:         title,
				TargetPackage: pkg,
				Milestone:     currentMilestone,
				Status:        st,
			}
			data.Tickets = append(data.Tickets, ticket)
		}
	}

	for i := range data.Tickets {
		t := &data.Tickets[i]
		if ts, ok := tableSpecs[t.ID]; ok {
			t.FeatureID = ts.featID
			t.StoryPoints = ts.sp
			t.ComplexityScore = ts.complexity
		}
	}

	parseDetailedSpecs(content, data)
	return data, scanner.Err()
}

func parseDetailedSpecs(content string, data *DevStatusData) {
	lines := strings.Split(content, "\n")
	specMap := make(map[string]struct {
		targetPkg string
		specs     string
		goal      string
		criteria  []string
	})

	var currentID string
	var currentPkg, currentSpecs, currentGoal string
	var currentCriteria []string

	commitTicket := func() {
		if currentID != "" {
			specMap[currentID] = struct {
				targetPkg string
				specs     string
				goal      string
				criteria  []string
			}{currentPkg, currentSpecs, currentGoal, currentCriteria}
		}
		currentID = ""
		currentPkg = ""
		currentSpecs = ""
		currentGoal = ""
		currentCriteria = nil
	}

	ticketHdrRegex := regexp.MustCompile(`^###\s+Ticket\s+` + "`" + `?(DEN-\d+)` + "`" + `?:`)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if m := ticketHdrRegex.FindStringSubmatch(trimmed); len(m) > 1 {
			commitTicket()
			currentID = m[1]
			continue
		}
		if currentID != "" {
			if strings.HasPrefix(trimmed, "## ") || (strings.HasPrefix(trimmed, "### ") && !ticketHdrRegex.MatchString(trimmed)) {
				commitTicket()
				continue
			}
			switch {
			case strings.HasPrefix(trimmed, "- **Target Package**:"):
				currentPkg = strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "- **Target Package**:")), "`")
			case strings.HasPrefix(trimmed, "- **Governing Specs**:"):
				currentSpecs = strings.TrimSpace(strings.TrimPrefix(trimmed, "- **Governing Specs**:"))
			case strings.HasPrefix(trimmed, "- **Goal**:"):
				currentGoal = strings.TrimSpace(strings.TrimPrefix(trimmed, "- **Goal**:"))
			case strings.HasPrefix(trimmed, "- [ ]") || strings.HasPrefix(trimmed, "- [x]"):
				currentCriteria = append(currentCriteria, trimmed)
			}
		}
	}
	commitTicket()

	for i := range data.Tickets {
		t := &data.Tickets[i]
		if s, ok := specMap[t.ID]; ok {
			if t.TargetPackage == "" {
				t.TargetPackage = s.targetPkg
			}
			t.GoverningSpecs = s.specs
			t.Goal = s.goal
			t.AcceptanceCriteria = s.criteria
		}
	}
}

// PushStatusToPlane moves all DEV_STATUS content to Plane service, generating Gantt dates and blocker relations.
func PushStatusToPlane(ctx context.Context, client *Client, mdContent string) (*StatusReplicationReport, error) {
	return PushStatusToPlaneIn(ctx, client, mdContent, ".")
}
