package synapse

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func fixture3Members2Contracts() *Synapse {
	return &Synapse{
		ID:        "web-api",
		Name:      "Web \"API\"",
		State:     StateActive,
		CreatedAt: time.Date(2026, 10, 18, 8, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 10, 18, 8, 30, 0, 0, time.UTC),
		Sync: Sync{
			Strategy: "go.work",
		},
		Parity: Parity{
			Commands: map[string]string{
				"api-gateway": "go test ./contract/...",
				"client.web":  "bun test",
			},
		},
		Members: []Member{
			{
				ProjectID: "dpid_0123456789abcdef0123456789abcdef",
				Alias:     "client-web",
				Role:      RoleConsumer,
				PathHint:  "/abs/path/client",
			},
			{
				ProjectID: "dpid_abcdef0123456789abcdef0123456789",
				Alias:     "api-gateway",
				Role:      RoleProducer,
				PathHint:  "/abs/path/gateway",
			},
			{
				ProjectID: "dpid_9876543210fedcba9876543210fedcba",
				Alias:     "core-engine",
				Role:      RolePeer,
				PathHint:  "/abs/path/core",
			},
		},
		Contracts: []Contract{
			{
				Kind:     "openapi",
				Producer: "api-gateway",
				Path:     "api/openapi.yaml",
				Consumers: []Consumer{
					{
						Alias: "client-web",
						Path:  "src/api/types.ts",
					},
				},
			},
			{
				Kind:     "proto",
				Producer: "core-engine",
				Path:     "proto/service.proto",
				Consumers: []Consumer{
					{
						Alias: "api-gateway",
						Path:  "pkg/pb/service.pb.go",
					},
					{
						Alias: "client-web",
						Path:  "src/pb/service.ts",
					},
				},
			},
		},
	}
}

func TestRoundTrip3Members2Contracts(t *testing.T) {
	s := fixture3Members2Contracts()

	marshaled, err := Marshal(s)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	content := string(marshaled)

	// Verify exact on-disk layout requirements
	expectedSnippets := []string{
		`id = "web-api"`,
		`name = "Web \"API\""`,
		`state = "active"`,
		`created_at = "2026-10-18T08:00:00Z"`,
		`updated_at = "2026-10-18T08:30:00Z"`,
		"[sync]\nstrategy = \"go.work\"",
		"[parity.commands]\napi-gateway = \"go test ./contract/...\"\n\"client.web\" = \"bun test\"",
		"[[members]]\nproject_id = \"dpid_0123456789abcdef0123456789abcdef\"\nalias = \"client-web\"\nrole = \"consumer\"\npath_hint = \"/abs/path/client\"",
		"[[contracts]]\nkind = \"openapi\"\nproducer = \"api-gateway\"\npath = \"api/openapi.yaml\"",
		"[[contracts.consumers]]\nalias = \"client-web\"\npath = \"src/api/types.ts\"",
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(content, snippet) {
			t.Fatalf("marshaled output missing expected snippet:\n%s\n\nFull content:\n%s", snippet, content)
		}
	}

	unmarshaled, err := Unmarshal(marshaled)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !reflect.DeepEqual(unmarshaled, s) {
		t.Fatalf("round-trip deep equality failed.\nExpected: %+v\nGot: %+v", s, unmarshaled)
	}
}

func TestMarshalOmitEmptySections(t *testing.T) {
	s := &Synapse{
		ID:        "minimal",
		Name:      "Minimal Link",
		State:     StateDraft,
		CreatedAt: time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC),
	}

	marshaled, err := Marshal(s)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	content := string(marshaled)
	forbidden := []string{"[sync]", "[parity.commands]", "[[members]]", "[[contracts]]"}
	for _, f := range forbidden {
		if strings.Contains(content, f) {
			t.Fatalf("empty section %q should have been omitted, got:\n%s", f, content)
		}
	}

	unmarshaled, err := Unmarshal(marshaled)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if unmarshaled.ID != s.ID || unmarshaled.Name != s.Name || unmarshaled.State != s.State {
		t.Fatalf("unmarshaled minimal synapse does not match original: %+v vs %+v", unmarshaled, s)
	}
}

func TestMarshalNil(t *testing.T) {
	if _, err := Marshal(nil); err == nil {
		t.Fatalf("expected error marshaling nil synapse, got nil")
	}
}

func TestCommentsAndWhitespace(t *testing.T) {
	input := `
# Header comment
id = "test-synapse" # inline comment

name = "Link # Not a comment" # Real inline comment
state = "draft"

# Intermediate comment
[sync]
strategy = "none" # comment here

[[members]]
# Member comment
project_id = "dpid_11111111111111111111111111111111"
alias = "producer-repo"
role = "producer"
path_hint = "/path/producer"

[[members]]
project_id = "dpid_22222222222222222222222222222222"
alias = "consumer-repo"
role = "consumer"
path_hint = "/path/consumer"
`
	s, err := Unmarshal([]byte(input))
	if err != nil {
		t.Fatalf("Unmarshal with comments failed: %v", err)
	}

	if s.ID != "test-synapse" {
		t.Fatalf("expected ID 'test-synapse', got %q", s.ID)
	}
	if s.Name != "Link # Not a comment" {
		t.Fatalf("expected Name 'Link # Not a comment', got %q", s.Name)
	}
	if s.Sync.Strategy != "none" {
		t.Fatalf("expected sync strategy 'none', got %q", s.Sync.Strategy)
	}
	if len(s.Members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(s.Members))
	}
}

func TestStringEscapes(t *testing.T) {
	input := `
id = "escape-test"
name = "Line1\nLine2\tTabbed\\Backslash\"Quoted\"\u0041\u2194\U00000042\rReturn"
state = "draft"
`
	s, err := Unmarshal([]byte(input))
	if err != nil {
		t.Fatalf("Unmarshal with escapes failed: %v", err)
	}

	expectedName := "Line1\nLine2\tTabbed\\Backslash\"Quoted\"A↔B\rReturn"
	if s.Name != expectedName {
		t.Fatalf("escaped name mismatch.\nExpected: %q\nGot:      %q", expectedName, s.Name)
	}

	// Re-marshal and ensure it produces round-trippable output
	data, err := Marshal(s)
	if err != nil {
		t.Fatalf("Marshal with escapes failed: %v", err)
	}
	s2, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal re-marshaled failed: %v", err)
	}
	if s2.Name != expectedName {
		t.Fatalf("round-trip escaped name mismatch: %q vs %q", s2.Name, expectedName)
	}
}

func TestLineNumberedErrors(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectedErr string
	}{
		{
			name: "unknown_root_key",
			input: `id = "foo"
unknown_field = "bar"
`,
			expectedErr: `line 2: unknown key "unknown_field" in root table`,
		},
		{
			name: "unknown_sync_key",
			input: `id = "foo"
[sync]
bad_sync = "go.work"
`,
			expectedErr: `line 3: unknown key "bad_sync" in [sync]`,
		},
		{
			name: "unknown_member_key",
			input: `id = "foo"
[[members]]
project_id = "dpid_1"
alias = "a"
extra = "bad"
`,
			expectedErr: `line 5: unknown key "extra" in [[members]]`,
		},
		{
			name: "unknown_contract_key",
			input: `id = "foo"
[[contracts]]
kind = "proto"
invalid_prop = "x"
`,
			expectedErr: `line 4: unknown key "invalid_prop" in [[contracts]]`,
		},
		{
			name: "unknown_consumer_key",
			input: `id = "foo"
[[contracts]]
kind = "proto"
producer = "a"
path = "p"
[[contracts.consumers]]
alias = "b"
extra_consumer = "err"
`,
			expectedErr: `line 8: unknown key "extra_consumer" in [[contracts.consumers]]`,
		},
		{
			name: "unknown_table",
			input: `id = "foo"
[unknown_section]
`,
			expectedErr: `line 2: unknown table "unknown_section"`,
		},
		{
			name: "unknown_array_of_tables",
			input: `id = "foo"
[[unknown_array]]
`,
			expectedErr: `line 2: unknown array of tables "unknown_array"`,
		},
		{
			name: "malformed_single_table_header",
			input: `id = "foo"
[sync
`,
			expectedErr: `line 2: malformed table header: [sync`,
		},
		{
			name: "malformed_array_table_header",
			input: `id = "foo"
[[members]
`,
			expectedErr: `line 2: malformed array of tables header: [[members]`,
		},
		{
			name: "missing_equals",
			input: `id = "foo"
malformed line without equals
`,
			expectedErr: `line 2: missing '=' in key-value pair`,
		},
		{
			name: "invalid_bare_key",
			input: `id = "foo"
invalid.bare.key = "val"
`,
			expectedErr: `line 2: invalid bare key "invalid.bare.key"`,
		},
		{
			name: "duplicate_root_key",
			input: `id = "foo"
id = "bar"
`,
			expectedErr: `line 2: duplicate key "id" in root table`,
		},
		{
			name: "duplicate_single_table",
			input: `id = "foo"
[sync]
strategy = "go.work"
[sync]
strategy = "none"
`,
			expectedErr: `line 4: duplicate table "sync"`,
		},
		{
			name: "consumer_before_contract",
			input: `id = "foo"
[[contracts.consumers]]
alias = "client"
`,
			expectedErr: `line 2: [[contracts.consumers]] declared before any [[contracts]]`,
		},
		{
			name: "unclosed_string_in_val",
			input: `id = "foo
name = "bar"
`,
			expectedErr: `line 1: unclosed string literal`,
		},
		{
			name:        "unterminated_escape_sequence",
			input:       `id = "foo\`,
			expectedErr: `line 1: unclosed string literal`,
		},
		{
			name:        "short_unicode_escape",
			input:       `id = "\u12"`,
			expectedErr: `line 1: short \u escape sequence`,
		},
		{
			name:        "invalid_hex_unicode_escape",
			input:       `id = "\uGGGG"`,
			expectedErr: `line 1: invalid \u escape: GGGG`,
		},
		{
			name:        "short_capital_u_escape",
			input:       `id = "\U1234"`,
			expectedErr: `line 1: short \U escape sequence`,
		},
		{
			name:        "invalid_hex_capital_u_escape",
			input:       `id = "\U000000G1"`,
			expectedErr: `line 1: invalid \U escape: 000000G1`,
		},
		{
			name:        "unknown_escape_sequence",
			input:       `id = "\z"`,
			expectedErr: `line 1: unknown escape sequence \z`,
		},
		{
			name:        "unquoted_string_value",
			input:       `id = bare_value`,
			expectedErr: `line 1: expected double-quoted string value, got bare_value`,
		},
		{
			name:        "unexpected_trailing_chars_after_string",
			input:       `id = "foo" extra`,
			expectedErr: `line 1: unexpected characters after string: extra`,
		},
		{
			name:        "invalid_created_at",
			input:       `created_at = "not-a-timestamp"`,
			expectedErr: `line 1: invalid created_at timestamp "not-a-timestamp"`,
		},
		{
			name:        "invalid_updated_at",
			input:       `updated_at = "not-a-timestamp"`,
			expectedErr: `line 1: invalid updated_at timestamp "not-a-timestamp"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Unmarshal([]byte(tc.input))
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.expectedErr)
			}
			if !strings.Contains(err.Error(), tc.expectedErr) {
				t.Fatalf("expected error to contain %q, got %q", tc.expectedErr, err.Error())
			}
		})
	}
}

func TestBareTimestampSupport(t *testing.T) {
	input := `
id = "bare-ts"
name = "Bare Timestamp"
state = "active"
created_at = 2026-10-18T08:00:00Z
updated_at = 2026-10-18T08:30:00Z
`
	s, err := Unmarshal([]byte(input))
	if err != nil {
		t.Fatalf("Unmarshal bare timestamp failed: %v", err)
	}

	expectedCreated := time.Date(2026, 10, 18, 8, 0, 0, 0, time.UTC)
	expectedUpdated := time.Date(2026, 10, 18, 8, 30, 0, 0, time.UTC)

	if !s.CreatedAt.Equal(expectedCreated) {
		t.Fatalf("created_at mismatch: expected %v, got %v", expectedCreated, s.CreatedAt)
	}
	if !s.UpdatedAt.Equal(expectedUpdated) {
		t.Fatalf("updated_at mismatch: expected %v, got %v", expectedUpdated, s.UpdatedAt)
	}
}

func TestMemberSourceFieldsRoundTrip(t *testing.T) {
	locked := time.Date(2026, 10, 18, 9, 30, 0, 0, time.UTC)
	in := &Synapse{
		ID: "stack", Name: "stack", State: StateActive,
		Members: []Member{
			{ProjectID: "dpid_a", Alias: "control", Role: RoleConsumer, PathHint: "/w/control"},
			{
				ProjectID: "dpid_b", Alias: "mcphub", Role: RoleProducer, PathHint: "/w/hub",
				Source: SourceRemote, Remote: "https://github.com/o/hub.git", Ref: "develop",
				Module: "dendrite.dev/mcphub", LockedSHA: "abc123", LockedAt: locked,
			},
		},
	}
	data, err := Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), "source = ") != 1 {
		t.Fatalf("empty optional member keys must be omitted:\n%s", data)
	}
	out, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal: %v\n%s", err, data)
	}
	if !reflect.DeepEqual(in.Members, out.Members) {
		t.Fatalf("members round trip:\n got %+v\nwant %+v", out.Members, in.Members)
	}
	if !out.Members[1].IsRemote() || out.Members[0].IsRemote() {
		t.Fatal("IsRemote mismatch")
	}

	bad := strings.Replace(string(data), `locked_at = "2026-10-18T09:30:00Z"`, `locked_at = "yesterday"`, 1)
	if _, err := Unmarshal([]byte(bad)); err == nil || !strings.Contains(err.Error(), "locked_at") {
		t.Fatalf("expected locked_at parse error, got %v", err)
	}
	bare := strings.Replace(string(data), `locked_at = "2026-10-18T09:30:00Z"`, `locked_at = 2026-10-18T09:30:00Z`, 1)
	if _, err := Unmarshal([]byte(bare)); err != nil {
		t.Fatalf("bare RFC3339 locked_at must parse: %v", err)
	}
}

func TestValidateMemberSource(t *testing.T) {
	base := func(m Member) *Synapse {
		return &Synapse{ID: "s", Members: []Member{
			{ProjectID: "dpid_a", Alias: "a", Role: RolePeer},
			m,
		}}
	}
	cases := []struct {
		m       Member
		wantErr string
	}{
		{Member{ProjectID: "dpid_b", Alias: "b", Role: RolePeer, Source: SourceLocal}, ""},
		{Member{ProjectID: "dpid_b", Alias: "b", Role: RolePeer, Source: SourceRemote, Remote: "u"}, ""},
		{Member{ProjectID: "dpid_b", Alias: "b", Role: RolePeer, Source: SourceRemote}, "requires a remote URL"},
		{Member{ProjectID: "dpid_b", Alias: "b", Role: RolePeer, Source: "vendored"}, "invalid source"},
	}
	for _, c := range cases {
		err := base(c.m).Validate()
		if c.wantErr == "" && err != nil {
			t.Errorf("%+v: unexpected %v", c.m, err)
		}
		if c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)) {
			t.Errorf("%+v: err = %v, want %q", c.m, err, c.wantErr)
		}
	}
}
