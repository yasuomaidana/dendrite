package token_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"Dendrite/pkg/scheduler/token"
)

func TestDefaultPolicy(t *testing.T) {
	p := token.DefaultPolicy()
	if err := p.Validate(); err != nil {
		t.Fatalf("default policy failed validation: %v", err)
	}

	if p.NightWindow != "23:00-06:00" {
		t.Errorf("unexpected night window: %s", p.NightWindow)
	}
	if p.WeeklyReset != "SUN 23:59" {
		t.Errorf("unexpected weekly reset: %s", p.WeeklyReset)
	}
	if p.SprintBeforeReset != 48*time.Hour {
		t.Errorf("unexpected sprint duration: %v", p.SprintBeforeReset)
	}
	if p.ReserveFloor != 0.20 {
		t.Errorf("unexpected reserve floor: %f", p.ReserveFloor)
	}
	if p.AllowZero != false {
		t.Errorf("expected allow_zero to be false")
	}
	if p.Mode != "auto" {
		t.Errorf("unexpected mode: %s", p.Mode)
	}
}

func TestParseNightWindow(t *testing.T) {
	tests := []struct {
		input      string
		wantStartH int
		wantStartM int
		wantEndH   int
		wantEndM   int
		wantErr    bool
	}{
		{"23:00-06:00", 23, 0, 6, 0, false},
		{"01:30-05:45", 1, 30, 5, 45, false},
		{"00:00-23:59", 0, 0, 23, 59, false},
		{"invalid", 0, 0, 0, 0, true},
		{"25:00-06:00", 0, 0, 0, 0, true},
		{"23:61-06:00", 0, 0, 0, 0, true},
		{"23:00-24:00", 0, 0, 0, 0, true},
		{"23:00", 0, 0, 0, 0, true},
		{"23:aa-06:00", 0, 0, 0, 0, true},
		{"aa:00-06:00", 0, 0, 0, 0, true},
		{"23:00-aa:00", 0, 0, 0, 0, true},
		{"23:00-06:aa", 0, 0, 0, 0, true},
		{"23:00-06:65", 0, 0, 0, 0, true},
	}

	for _, tt := range tests {
		sh, sm, eh, em, err := token.ParseNightWindow(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseNightWindow(%q) err=%v, wantErr=%v", tt.input, err, tt.wantErr)
			continue
		}
		if !tt.wantErr {
			if sh != tt.wantStartH || sm != tt.wantStartM || eh != tt.wantEndH || em != tt.wantEndM {
				t.Errorf("ParseNightWindow(%q) = (%d,%d,%d,%d), want (%d,%d,%d,%d)",
					tt.input, sh, sm, eh, em, tt.wantStartH, tt.wantStartM, tt.wantEndH, tt.wantEndM)
			}
		}
	}
}

func TestParseWeeklyReset(t *testing.T) {
	tests := []struct {
		input       string
		wantWeekday time.Weekday
		wantHour    int
		wantMin     int
		wantErr     bool
	}{
		{"SUN 23:59", time.Sunday, 23, 59, false},
		{"SUNDAY 12:00", time.Sunday, 12, 0, false},
		{"MON 08:30", time.Monday, 8, 30, false},
		{"FRIDAY 18:00", time.Friday, 18, 0, false},
		{"WED 00:00", time.Wednesday, 0, 0, false},
		{"SAT 23:59", time.Saturday, 23, 59, false},
		{"TUE 14:15", time.Tuesday, 14, 15, false},
		{"THU 10:10", time.Thursday, 10, 10, false},
		{"INVALID 12:00", 0, 0, 0, true},
		{"SUN 25:00", 0, 0, 0, true},
		{"SUN 12:65", 0, 0, 0, true},
		{"SUN 12:aa", 0, 0, 0, true},
		{"SUN aa:00", 0, 0, 0, true},
		{"SUN 12", 0, 0, 0, true},
		{"SUN", 0, 0, 0, true},
	}

	for _, tt := range tests {
		w, h, m, err := token.ParseWeeklyReset(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseWeeklyReset(%q) err=%v, wantErr=%v", tt.input, err, tt.wantErr)
			continue
		}
		if !tt.wantErr {
			if w != tt.wantWeekday || h != tt.wantHour || m != tt.wantMin {
				t.Errorf("ParseWeeklyReset(%q) = (%v,%d,%d), want (%v,%d,%d)",
					tt.input, w, h, m, tt.wantWeekday, tt.wantHour, tt.wantMin)
			}
		}
	}
}

func TestPolicyValidation(t *testing.T) {
	valid := token.DefaultPolicy()

	// Bad night window
	badWindow := valid
	badWindow.NightWindow = "invalid"
	if err := badWindow.Validate(); err == nil {
		t.Errorf("expected error for invalid night window")
	}

	// Bad weekly reset
	badReset := valid
	badReset.WeeklyReset = "invalid"
	if err := badReset.Validate(); err == nil {
		t.Errorf("expected error for invalid weekly reset")
	}

	// Negative sprint duration
	badSprint := valid
	badSprint.SprintBeforeReset = -1 * time.Hour
	if err := badSprint.Validate(); err == nil {
		t.Errorf("expected error for negative sprint duration")
	}

	// Bad reserve floor
	badFloor := valid
	badFloor.ReserveFloor = 1.5
	if err := badFloor.Validate(); err == nil {
		t.Errorf("expected error for reserve floor > 1.0")
	}

	badFloorNeg := valid
	badFloorNeg.ReserveFloor = -0.1
	if err := badFloorNeg.Validate(); err == nil {
		t.Errorf("expected error for reserve floor < 0.0")
	}

	// Bad mode
	badMode := valid
	badMode.Mode = "unsupported"
	if err := badMode.Validate(); err == nil {
		t.Errorf("expected error for invalid mode")
	}
}

func TestPolicyPersistence(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "scheduler_policy_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	filePath := filepath.Join(tmpDir, "policy.json")

	// Load non-existent should return default
	p, err := token.LoadPolicy(filePath)
	if err != nil {
		t.Fatalf("failed to load default from missing file: %v", err)
	}
	if p.NightWindow != "23:00-06:00" {
		t.Errorf("expected default night window, got %s", p.NightWindow)
	}

	// Save invalid policy
	badPolicy := token.DefaultPolicy()
	badPolicy.Mode = "invalid"
	if err := token.SavePolicy(filePath, &badPolicy); err == nil {
		t.Errorf("expected error saving invalid policy")
	}

	// Corrupt JSON in file
	if err := os.WriteFile(filePath, []byte("{invalid-json"), 0644); err != nil {
		t.Fatalf("failed to write corrupted file: %v", err)
	}
	if _, err := token.LoadPolicy(filePath); err == nil {
		t.Errorf("expected error loading corrupted JSON policy")
	}

	// Invalid policy in file
	if err := os.WriteFile(filePath, []byte(`{"night_window":"invalid","weekly_reset":"SUN 23:59","sprint_before_reset":1000,"reserve_floor":0.2,"mode":"auto"}`), 0644); err != nil {
		t.Fatalf("failed to write invalid file: %v", err)
	}
	if _, err := token.LoadPolicy(filePath); err == nil {
		t.Errorf("expected error loading invalid policy")
	}

	// Mutate and save
	p.NightWindow = "00:00-05:00"
	p.WeeklyReset = "MON 06:00"
	p.AllowZero = true
	p.Mode = "always"

	if err := token.SavePolicy(filePath, p); err != nil {
		t.Fatalf("failed to save policy: %v", err)
	}

	// Reload and verify
	loaded, err := token.LoadPolicy(filePath)
	if err != nil {
		t.Fatalf("failed to reload policy: %v", err)
	}
	if loaded.NightWindow != "00:00-05:00" || loaded.WeeklyReset != "MON 06:00" || !loaded.AllowZero || loaded.Mode != "always" {
		t.Errorf("reloaded policy does not match saved: %+v", loaded)
	}
}

func TestEvaluator_Errors(t *testing.T) {
	tm := time.Now()
	badPolicy := token.Policy{NightWindow: "bad", WeeklyReset: "SUN 23:59", SprintBeforeReset: 48 * time.Hour, ReserveFloor: 0.2, Mode: "auto"}
	if _, err := token.IsNightWindow(tm, &badPolicy); err == nil {
		t.Errorf("expected error for bad night window in IsNightWindow")
	}
	if _, err := token.Evaluate(tm, &badPolicy, 1.0, 0.0); err == nil {
		t.Errorf("expected error evaluating bad policy")
	}

	badResetPolicy := token.Policy{NightWindow: "23:00-06:00", WeeklyReset: "bad", SprintBeforeReset: 48 * time.Hour, ReserveFloor: 0.2, Mode: "auto"}
	if _, err := token.NextWeeklyReset(tm, &badResetPolicy); err == nil {
		t.Errorf("expected error for bad weekly reset")
	}
	if _, err := token.TimeUntilReset(tm, &badResetPolicy); err == nil {
		t.Errorf("expected error for TimeUntilReset with bad reset")
	}
	if _, err := token.IsSprintWindow(tm, &badResetPolicy); err == nil {
		t.Errorf("expected error for IsSprintWindow with bad reset")
	}
}
