package synapse

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestQuoteStringControlCharacters(t *testing.T) {
	s := fixture3Members2Contracts()
	s.Name = "bell\x07tab\tcr\r"
	data, err := Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `\u0007`) {
		t.Fatalf("control char not escaped as \\u0007:\n%s", data)
	}
	got, err := Unmarshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != s.Name {
		t.Fatalf("round-trip name = %q, want %q", got.Name, s.Name)
	}
}

func TestParseQuotedStringEdgeCases(t *testing.T) {
	cases := map[string]string{
		"not quoted":      `abc`,
		"dangling escape": `"abc\`,
		"unclosed":        `"abc`,
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, err := parseQuotedString(in); err == nil {
				t.Fatalf("parseQuotedString(%q) succeeded, want error", in)
			}
		})
	}
}

func TestUnmarshalRejectsMalformedLines(t *testing.T) {
	cases := map[string]string{
		"bad quoted key":      "\"id",
		"quoted key no equal": "\"id\" \"x\"",
		"empty value":         "id =",
		"dup sync key":        "[sync]\nstrategy = \"none\"\nstrategy = \"none\"",
		"dup parity key":      "[parity.commands]\na = \"x\"\na = \"y\"",
		"dup member key":      "[[members]]\nalias = \"a\"\nalias = \"b\"",
		"dup contract key":    "[[contracts]]\nkind = \"go\"\nkind = \"go\"",
		"dup consumer key":    "[[contracts]]\nkind = \"go\"\n[[contracts.consumers]]\nalias = \"a\"\nalias = \"b\"",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Unmarshal([]byte(in))
			if err == nil {
				t.Fatalf("Unmarshal(%q) succeeded, want error", in)
			}
			if !strings.Contains(err.Error(), "line ") {
				t.Fatalf("error %q lacks a line number", err)
			}
		})
	}
}

func TestStoreFilesystemErrors(t *testing.T) {
	t.Run("default store without home", func(t *testing.T) {
		t.Setenv("DENDRITE_HOME", "")
		t.Setenv("HOME", "")
		if _, err := DefaultStore(); err == nil {
			t.Fatal("expected DefaultStore to fail without a home directory")
		}
	})

	t.Run("default store honours DENDRITE_HOME", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("DENDRITE_HOME", home)
		st, err := DefaultStore()
		if err != nil {
			t.Fatal(err)
		}
		if st.Dir() != filepath.Join(home, DirName) {
			t.Fatalf("Dir() = %s", st.Dir())
		}
	})

	t.Run("store dir is a file", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "synapses")
		if err := os.WriteFile(file, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		st := NewStore(file)
		if _, err := st.List(); err == nil {
			t.Fatal("List on a file should fail")
		}
		if _, err := st.ReferencingProject("dpid_x"); err == nil {
			t.Fatal("ReferencingProject should propagate List errors")
		}
	})

	t.Run("manifest path is a directory", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, "web-api.toml", "x"), 0o755); err != nil {
			t.Fatal(err)
		}
		st := NewStore(dir)
		if _, err := st.Load("web-api"); err == nil || err == ErrNotFound {
			t.Fatalf("Load on a directory: got %v, want read error", err)
		}
		if err := st.Delete("web-api"); err == nil || err == ErrNotFound {
			t.Fatalf("Delete of non-empty directory: got %v, want remove error", err)
		}
	})

	t.Run("unreadable manifest", func(t *testing.T) {
		dir := t.TempDir()
		f := filepath.Join(dir, "x.toml")
		if err := os.WriteFile(f, []byte("id = \"x\"\n"), 0o000); err != nil {
			t.Fatal(err)
		}
		if _, err := NewStore(dir).List(); err == nil || !strings.Contains(err.Error(), "x.toml") {
			t.Fatalf("List with unreadable file: got %v, want error naming x.toml", err)
		}
	})
}

func TestParseKeyAndValueUnclosedQuotedKey(t *testing.T) {
	if _, _, err := parseKeyAndValue(`"unclosed`); err == nil {
		t.Fatal("expected error for a quoted key that never closes")
	}
}
