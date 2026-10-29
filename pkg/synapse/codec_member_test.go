package synapse

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCodecMemberOptionalFieldsAndChannel(t *testing.T) {
	lockedTime := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	s := &Synapse{
		ID:        "full-synapse",
		Name:      "Full Synapse with Control \x05 Char",
		State:     StateActive,
		CreatedAt: time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 10, 20, 11, 0, 0, 0, time.UTC),
		Channel:   ChannelDev,
		Members: []Member{
			{
				ProjectID:    "dpid_0123456789abcdef0123456789abcdef",
				Alias:        "remote-producer",
				Role:         RoleProducer,
				PathHint:     "/path/remote",
				Source:       SourceRemote,
				Remote:       "https://github.com/org/repo.git",
				Ref:          "develop",
				Module:       "github.com/org/repo",
				LockedSHA:    "abc123def456",
				LockedAt:     lockedTime,
				PlaneProject: "DPROD",
			},
			{
				ProjectID: "dpid_fedcba9876543210fedcba9876543210",
				Alias:     "local-consumer",
				Role:      RoleConsumer,
				PathHint:  "/path/local",
			},
		},
		Sync: Sync{
			Strategy: "cargo-patch",
		},
	}

	marshaled, err := Marshal(s)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	content := string(marshaled)
	expectedSubstrings := []string{
		`channel = "dev"`,
		`source = "remote"`,
		`remote = "https://github.com/org/repo.git"`,
		`ref = "develop"`,
		`module = "github.com/org/repo"`,
		`locked_sha = "abc123def456"`,
		`locked_at = "2026-10-20T12:00:00Z"`,
		`plane_project = "DPROD"`,
		`\u0005`, // control char escape from quoteString
	}
	for _, sub := range expectedSubstrings {
		if !strings.Contains(content, sub) {
			t.Fatalf("expected marshaled output to contain %q, got:\n%s", sub, content)
		}
	}

	unmarshaled, err := Unmarshal(marshaled)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if unmarshaled.Channel != ChannelDev {
		t.Errorf("expected ChannelDev, got %s", unmarshaled.Channel)
	}

	m0 := unmarshaled.Members[0]
	if m0.Source != SourceRemote || m0.Remote != "https://github.com/org/repo.git" ||
		m0.Ref != "develop" || m0.Module != "github.com/org/repo" ||
		m0.LockedSHA != "abc123def456" || !m0.LockedAt.Equal(lockedTime) ||
		m0.PlaneProject != "DPROD" {
		t.Errorf("unmarshaled member fields mismatch: %+v", m0)
	}
}

func TestUnmarshalInvalidLockedAt(t *testing.T) {
	input := `id = "invalid-locked-at"
name = "Invalid Locked At"
state = "draft"

[[members]]
project_id = "dpid_11111111111111111111111111111111"
alias = "producer"
role = "producer"
path_hint = "/p"
locked_at = "not-a-valid-timestamp"
`
	_, err := Unmarshal([]byte(input))
	if err == nil {
		t.Fatal("expected error on invalid locked_at timestamp, got nil")
	}
	if !strings.Contains(err.Error(), "invalid locked_at timestamp") {
		t.Fatalf("expected 'invalid locked_at timestamp' in error, got: %v", err)
	}
}

func TestStoreReferencingProjectError(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir)

	// Add corrupted file so List fails
	if err := os.WriteFile(filepath.Join(dir, "bad.toml"), []byte("corrupt = ="), 0644); err != nil {
		t.Fatalf("failed to write bad.toml: %v", err)
	}

	_, err := st.ReferencingProject("dpid_00000000000000000000000000000000")
	if err == nil {
		t.Fatal("expected error when store.List fails in ReferencingProject, got nil")
	}
}
