package token

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// DefaultPolicyFile specifies the default configuration path for scheduler policy.
const DefaultPolicyFile = ".dendrite/scheduler_policy.json"

// DefaultPolicy returns the standard token-aware scheduling configuration.
func DefaultPolicy() Policy {
	return Policy{
		NightWindow:       "23:00-06:00",
		WeeklyReset:       "SUN 23:59",
		SprintBeforeReset: 48 * time.Hour,
		ReserveFloor:      0.20,
		AllowZero:         false,
		Mode:              "auto",
	}
}

// ParseNightWindow parses a window string in "HH:MM-HH:MM" format.
func ParseNightWindow(windowStr string) (startHour, startMin, endHour, endMin int, err error) {
	parts := strings.Split(strings.TrimSpace(windowStr), "-")
	if len(parts) != 2 {
		return 0, 0, 0, 0, fmt.Errorf("%w: night window format must be 'HH:MM-HH:MM', got '%s'", ErrInvalidPolicy, windowStr)
	}

	startParts := strings.Split(parts[0], ":")
	endParts := strings.Split(parts[1], ":")
	if len(startParts) != 2 || len(endParts) != 2 {
		return 0, 0, 0, 0, fmt.Errorf("%w: night window times must be 'HH:MM', got '%s'", ErrInvalidPolicy, windowStr)
	}

	startHour, err = strconv.Atoi(startParts[0])
	if err != nil || startHour < 0 || startHour > 23 {
		return 0, 0, 0, 0, fmt.Errorf("%w: invalid start hour in window '%s'", ErrInvalidPolicy, windowStr)
	}

	startMin, err = strconv.Atoi(startParts[1])
	if err != nil || startMin < 0 || startMin > 59 {
		return 0, 0, 0, 0, fmt.Errorf("%w: invalid start minute in window '%s'", ErrInvalidPolicy, windowStr)
	}

	endHour, err = strconv.Atoi(endParts[0])
	if err != nil || endHour < 0 || endHour > 23 {
		return 0, 0, 0, 0, fmt.Errorf("%w: invalid end hour in window '%s'", ErrInvalidPolicy, windowStr)
	}

	endMin, err = strconv.Atoi(endParts[1])
	if err != nil || endMin < 0 || endMin > 59 {
		return 0, 0, 0, 0, fmt.Errorf("%w: invalid end minute in window '%s'", ErrInvalidPolicy, windowStr)
	}

	return startHour, startMin, endHour, endMin, nil
}

// ParseWeeklyReset parses weekly reset strings like "SUN 23:59", "FRIDAY 18:00".
func ParseWeeklyReset(resetStr string) (time.Weekday, int, int, error) {
	fields := strings.Fields(strings.TrimSpace(resetStr))
	if len(fields) != 2 {
		return 0, 0, 0, fmt.Errorf("%w: weekly reset format must be 'DAY HH:MM', got '%s'", ErrInvalidPolicy, resetStr)
	}

	weekday, err := parseWeekday(fields[0])
	if err != nil {
		return 0, 0, 0, err
	}

	timeParts := strings.Split(fields[1], ":")
	if len(timeParts) != 2 {
		return 0, 0, 0, fmt.Errorf("%w: invalid reset time format '%s'", ErrInvalidPolicy, fields[1])
	}

	hour, err := strconv.Atoi(timeParts[0])
	if err != nil || hour < 0 || hour > 23 {
		return 0, 0, 0, fmt.Errorf("%w: invalid reset hour '%s'", ErrInvalidPolicy, timeParts[0])
	}

	min, err := strconv.Atoi(timeParts[1])
	if err != nil || min < 0 || min > 59 {
		return 0, 0, 0, fmt.Errorf("%w: invalid reset minute '%s'", ErrInvalidPolicy, timeParts[1])
	}

	return weekday, hour, min, nil
}

func parseWeekday(dayStr string) (time.Weekday, error) {
	switch strings.ToUpper(dayStr) {
	case "SUN", "SUNDAY":
		return time.Sunday, nil
	case "MON", "MONDAY":
		return time.Monday, nil
	case "TUE", "TUES", "TUESDAY":
		return time.Tuesday, nil
	case "WED", "WEDNESDAY":
		return time.Wednesday, nil
	case "THU", "THUR", "THURSDAY":
		return time.Thursday, nil
	case "FRI", "FRIDAY":
		return time.Friday, nil
	case "SAT", "SATURDAY":
		return time.Saturday, nil
	default:
		return 0, fmt.Errorf("%w: unrecognized weekday '%s'", ErrInvalidPolicy, dayStr)
	}
}

// Validate checks policy consistency.
func (p *Policy) Validate() error {
	if _, _, _, _, err := ParseNightWindow(p.NightWindow); err != nil {
		return err
	}
	if _, _, _, err := ParseWeeklyReset(p.WeeklyReset); err != nil {
		return err
	}
	if p.SprintBeforeReset <= 0 {
		return fmt.Errorf("%w: sprint duration must be positive", ErrInvalidPolicy)
	}
	if p.ReserveFloor < 0.0 || p.ReserveFloor > 1.0 {
		return fmt.Errorf("%w: reserve floor must be between 0.0 and 1.0, got %f", ErrInvalidPolicy, p.ReserveFloor)
	}

	switch strings.ToLower(p.Mode) {
	case "auto", "always", "night", "sprint", "paused":
		// valid
	default:
		return fmt.Errorf("%w: invalid mode '%s' (allowed: auto, always, night, sprint, paused)", ErrInvalidPolicy, p.Mode)
	}

	return nil
}

// LoadPolicy reads and deserializes the policy from a JSON file.
func LoadPolicy(path string) (*Policy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			def := DefaultPolicy()
			return &def, nil
		}
		return nil, fmt.Errorf("reading policy file: %w", err)
	}

	var p Policy
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("unmarshaling policy: %w", err)
	}

	if err := p.Validate(); err != nil {
		return nil, err
	}

	return &p, nil
}

// SavePolicy writes the policy to the destination path atomically.
func SavePolicy(path string, p *Policy) error {
	if err := p.Validate(); err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating policy directory: %w", err)
	}

	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling policy: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}
