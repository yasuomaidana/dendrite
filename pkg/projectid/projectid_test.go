package projectid

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func createGitRepo(t *testing.T, originURL string) string {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command("git", "init", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v, output: %s", err, string(out))
	}
	if originURL != "" {
		cmd = exec.Command("git", "-C", dir, "remote", "add", "origin", originURL)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git remote add failed: %v, output: %s", err, string(out))
		}
	}
	return dir
}

func TestPath(t *testing.T) {
	dir := filepath.Join("test", "dir")
	expected := filepath.Join(dir, ".dendrite", FileName)
	if got := Path(dir); got != expected {
		t.Errorf("Path(%q) = %q, want %q", dir, got, expected)
	}
}

func TestValid(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "valid id",
			input: "dpid_0123456789abcdef0123456789abcdef",
			want:  true,
		},
		{
			name:  "too short",
			input: "dpid_1234",
			want:  false,
		},
		{
			name:  "too long",
			input: "dpid_0123456789abcdef0123456789abcdef0",
			want:  false,
		},
		{
			name:  "missing prefix",
			input: "0123456789abcdef0123456789abcdef",
			want:  false,
		},
		{
			name:  "wrong prefix",
			input: "proj_0123456789abcdef0123456789abcdef",
			want:  false,
		},
		{
			name:  "uppercase hex",
			input: "dpid_0123456789ABCDEF0123456789ABCDEF",
			want:  false,
		},
		{
			name:  "invalid characters",
			input: "dpid_0123456789abcdef0123456789abcdeg",
			want:  false,
		},
		{
			name:  "empty string",
			input: "",
			want:  false,
		},
		{
			name:  "newline included",
			input: "dpid_0123456789abcdef0123456789abcdef\n",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Valid(tt.input); got != tt.want {
				t.Errorf("Valid(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestRead(t *testing.T) {
	t.Run("empty dir returns ErrNoProjectID", func(t *testing.T) {
		dir := t.TempDir()
		_, err := Read(dir)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrNoProjectID) {
			t.Fatalf("expected errors.Is(err, ErrNoProjectID), got %v", err)
		}
	})

	t.Run("malformed content returns error", func(t *testing.T) {
		dir := t.TempDir()
		idPath := Path(dir)
		if err := os.MkdirAll(filepath.Dir(idPath), 0755); err != nil {
			t.Fatal(err)
		}

		malformedInputs := []string{
			"malformed_id",
			"dpid_short",
			"",
			"dpid_0123456789ABCDEF0123456789ABCDEF",
		}

		for _, bad := range malformedInputs {
			if err := os.WriteFile(idPath, []byte(bad), 0644); err != nil {
				t.Fatal(err)
			}

			_, err := Read(dir)
			if err == nil {
				t.Errorf("Read() with content %q: expected error, got nil", bad)
			}
			if errors.Is(err, ErrNoProjectID) {
				t.Errorf("Read() with content %q: got ErrNoProjectID, expected malformed error", bad)
			}
		}
	})

	t.Run("valid file returns trimmed id", func(t *testing.T) {
		dir := t.TempDir()
		idPath := Path(dir)
		if err := os.MkdirAll(filepath.Dir(idPath), 0755); err != nil {
			t.Fatal(err)
		}

		validID := "dpid_0123456789abcdef0123456789abcdef"
		if err := os.WriteFile(idPath, []byte("  "+validID+" \n"), 0644); err != nil {
			t.Fatal(err)
		}

		got, err := Read(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != validID {
			t.Errorf("Read() = %q, want %q", got, validID)
		}
	})
}

func TestEnsure(t *testing.T) {
	t.Run("ensure twice returns same ID and file content unchanged", func(t *testing.T) {
		dir := t.TempDir()

		id1, err := Ensure(dir)
		if err != nil {
			t.Fatalf("Ensure(1) failed: %v", err)
		}
		if !Valid(id1) {
			t.Fatalf("Ensure(1) returned invalid ID: %q", id1)
		}

		idFile := Path(dir)
		content1, err := os.ReadFile(idFile)
		if err != nil {
			t.Fatalf("failed to read id file: %v", err)
		}
		expectedContent := id1 + "\n"
		if string(content1) != expectedContent {
			t.Errorf("file content = %q, want %q", string(content1), expectedContent)
		}

		id2, err := Ensure(dir)
		if err != nil {
			t.Fatalf("Ensure(2) failed: %v", err)
		}
		if id2 != id1 {
			t.Errorf("Ensure(2) = %q, want %q", id2, id1)
		}

		content2, err := os.ReadFile(idFile)
		if err != nil {
			t.Fatalf("failed to re-read id file: %v", err)
		}
		if string(content2) != string(content1) {
			t.Errorf("file content changed after second Ensure: got %q, want %q", string(content2), string(content1))
		}
	})

	t.Run("ensure on pre-existing valid file returns existing ID", func(t *testing.T) {
		dir := t.TempDir()
		idFile := Path(dir)
		if err := os.MkdirAll(filepath.Dir(idFile), 0755); err != nil {
			t.Fatal(err)
		}

		existingID := "dpid_deadbeefdeadbeefdeadbeefdeadbeef"
		if err := os.WriteFile(idFile, []byte(existingID+"\n"), 0644); err != nil {
			t.Fatal(err)
		}

		got, err := Ensure(dir)
		if err != nil {
			t.Fatalf("Ensure failed: %v", err)
		}
		if got != existingID {
			t.Errorf("Ensure = %q, want %q", got, existingID)
		}
	})

	t.Run("ensure on malformed file returns error", func(t *testing.T) {
		dir := t.TempDir()
		idFile := Path(dir)
		if err := os.MkdirAll(filepath.Dir(idFile), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(idFile, []byte("bad-content\n"), 0644); err != nil {
			t.Fatal(err)
		}

		_, err := Ensure(dir)
		if err == nil {
			t.Fatal("expected error on malformed file, got nil")
		}
	})
}

func TestNormalizeRemote(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "scp github",
			input: "git@github.com:A/b.git",
			want:  "github.com/a/b",
		},
		{
			name:  "https github",
			input: "https://github.com/A/b",
			want:  "github.com/a/b",
		},
		{
			name:  "ssh host with port",
			input: "ssh://git@host:22/a/b.git",
			want:  "host/a/b",
		},
		{
			name:  "empty remote",
			input: "",
			want:  "",
		},
		{
			name:  "whitespace only",
			input: "   ",
			want:  "",
		},
		{
			name:  "https gitlab with user and token",
			input: "https://user:tok@gitlab.com/g/s/r.git",
			want:  "gitlab.com/g/s/r",
		},
		{
			name:  "https github with trailing slash",
			input: "https://github.com/A/b/",
			want:  "github.com/a/b",
		},
		{
			name:  "scp github without dot git",
			input: "git@github.com:A/b",
			want:  "github.com/a/b",
		},
		{
			name:  "ssh without user",
			input: "ssh://host:22/a/b",
			want:  "host/a/b",
		},
		{
			name:  "ssh without port",
			input: "ssh://git@host/a/b.git",
			want:  "host/a/b",
		},
		{
			name:  "git scheme",
			input: "git://github.com/org/repo.git",
			want:  "github.com/org/repo",
		},
		{
			name:  "http gitlab trailing slash and dot git",
			input: "http://gitlab.com/group/subgroup/project.git/",
			want:  "gitlab.com/group/subgroup/project",
		},
		{
			name:  "leading and trailing spaces",
			input: "  https://github.com/A/b.git  ",
			want:  "github.com/a/b",
		},
		{
			name:  "host with scp leading slash",
			input: "host.com:/a/b.git",
			want:  "host.com/a/b",
		},
		{
			name:  "host without scp leading slash",
			input: "host.com:a/b.git",
			want:  "host.com/a/b",
		},
		{
			name:  "ipv6 with port",
			input: "ssh://[::1]:22/a/b.git",
			want:  "::1/a/b",
		},
		{
			name:  "ipv6 without port",
			input: "ssh://[::1]/a/b.git",
			want:  "::1/a/b",
		},
		{
			name:  "url with no path and port",
			input: "ssh://host:22",
			want:  "host",
		},
		{
			name:  "plain host and path",
			input: "host/a/b",
			want:  "host/a/b",
		},
		{
			name:  "plain host only",
			input: "host",
			want:  "host",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeRemote(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeRemote(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGitRemote(t *testing.T) {
	t.Run("non-repo returns empty string", func(t *testing.T) {
		dir := t.TempDir()
		if got := GitRemote(dir); got != "" {
			t.Errorf("GitRemote(non-repo) = %q, want empty string", got)
		}
	})

	t.Run("repo without origin returns empty string", func(t *testing.T) {
		dir := createGitRepo(t, "")
		if got := GitRemote(dir); got != "" {
			t.Errorf("GitRemote(no origin) = %q, want empty string", got)
		}
	})

	t.Run("repo with https origin returns URL", func(t *testing.T) {
		origin := "https://github.com/dendrite/synapse.git"
		dir := createGitRepo(t, origin)
		if got := GitRemote(dir); got != origin {
			t.Errorf("GitRemote() = %q, want %q", got, origin)
		}
	})

	t.Run("repo with ssh origin returns URL", func(t *testing.T) {
		origin := "git@github.com:dendrite/synapse.git"
		dir := createGitRepo(t, origin)
		if got := GitRemote(dir); got != origin {
			t.Errorf("GitRemote() = %q, want %q", got, origin)
		}
	})
}
