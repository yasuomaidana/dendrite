package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"Dendrite/pkg/env"
	"Dendrite/pkg/jetbrains"
)

func executeCommand(args ...string) (string, error) {
	rootCmd := buildRootCommand()
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs(args)

	err := rootCmd.ExecuteContext(context.Background())
	return buf.String(), err
}

func TestBuildRootCommand(t *testing.T) {
	rootCmd := buildRootCommand()
	if rootCmd == nil {
		t.Fatal("expected root command to be non-nil")
	}

	expectedSubcommands := []string{"version", "doctor", "env", "dev", "jetbrains"}
	for _, expected := range expectedSubcommands {
		cmd, _, err := rootCmd.Find([]string{expected})
		if err != nil || cmd == nil {
			t.Errorf("expected subcommand %q to exist", expected)
		}
	}
}

func TestVersionCommand(t *testing.T) {
	out, err := executeCommand("version")
	if err != nil {
		t.Fatalf("version command failed: %v", err)
	}

	expected := "Dendrite CLI v" + version
	if !strings.Contains(out, expected) {
		t.Errorf("expected output to contain %q, got %q", expected, out)
	}
}

func TestDoctorCommand(t *testing.T) {
	_, err := executeCommand("doctor")
	if err != nil {
		t.Fatalf("doctor command failed: %v", err)
	}
}

func TestEnvCommands(t *testing.T) {
	// Env/dev commands render stacks/ and status files relative to the CWD; never touch the repository.
	t.Chdir(t.TempDir())
	t.Run("env doctor", func(t *testing.T) {
		_, err := executeCommand("env", "doctor")
		if err != nil {
			t.Fatalf("env doctor failed: %v", err)
		}

	})

	t.Run("env plan defaults", func(t *testing.T) {
		_, err := executeCommand("env", "plan")
		if err != nil {
			t.Fatalf("env plan failed: %v", err)
		}

	})

	t.Run("env plan with flags", func(t *testing.T) {
		_, err := executeCommand("env", "plan", "--mode", "hybrid", "--with-linear")
		if err != nil {
			t.Fatalf("env plan with flags failed: %v", err)
		}

	})

	t.Run("env replicate dry-run", func(t *testing.T) {
		_, err := executeCommand("env", "replicate", "--mode", "container", "--dry-run")
		if err != nil {
			t.Fatalf("env replicate failed: %v", err)
		}

	})

	t.Run("env install target", func(t *testing.T) {
		_, err := executeCommand("env", "install", "ollama", "--mode", "container")
		if err != nil {
			t.Fatalf("env install failed: %v", err)
		}

	})

	t.Run("env status", func(t *testing.T) {
		_, err := executeCommand("env", "status")
		if err != nil {
			t.Fatalf("env status failed: %v", err)
		}

	})

	t.Run("env detector error branches", func(t *testing.T) {
		orig := hostDetectorProvider
		defer func() { hostDetectorProvider = orig }()

		hostDetectorProvider = func() HostDetector {
			return &failingDetector{}
		}

		_, _ = executeCommand("env", "doctor")
		_, _ = executeCommand("env", "plan")
		_, _ = executeCommand("env", "replicate")
		_, _ = executeCommand("env", "install", "plane")
		_, _ = executeCommand("env", "status")
	})
}

type failingDetector struct{}

func (f *failingDetector) Detect(ctx context.Context) (*env.HostInfo, error) {
	return nil, errors.New("simulated detector failure")
}

func TestDevCommands(t *testing.T) {
	// Env/dev commands render stacks/ and status files relative to the CWD; never touch the repository.
	t.Chdir(t.TempDir())
	t.Run("dev ollama default", func(t *testing.T) {
		_, err := executeCommand("dev", "ollama")
		if err != nil {
			t.Errorf("dev ollama failed: %v", err)
		}
	})

	t.Run("dev ollama custom endpoint", func(t *testing.T) {
		_, err := executeCommand("dev", "ollama", "http://localhost:11434")
		if err != nil {
			t.Errorf("dev ollama custom endpoint failed: %v", err)
		}
	})

	t.Run("dev ollama probe flag", func(t *testing.T) {
		_, err := executeCommand("dev", "ollama", "probe", "http://localhost:11434")
		if err != nil {
			t.Errorf("dev ollama probe failed: %v", err)
		}
	})

	t.Run("dev ollama unreachable", func(t *testing.T) {
		_, err := executeCommand("dev", "ollama", "http://127.0.0.1:59999")
		if err != nil {
			t.Errorf("dev ollama unreachable returned unexpected command error: %v", err)
		}
	})

	t.Run("dev audit-prompt", func(t *testing.T) {
		_, err := executeCommand("dev", "audit-prompt", "hello", "world", "autonomous", "system")
		if err != nil {
			t.Errorf("dev audit-prompt failed: %v", err)
		}
	})

	t.Run("dev ticket list and set", func(t *testing.T) {
		tmpDir := t.TempDir()
		statusFile := filepath.Join(tmpDir, "DEV_STATUS.md")
		initialContent := "# Project Status\n\n### Milestone 1\n- [ ] **DEN-001**: Test Ticket (`pkg/test`) *(TODO)*\n"
		if err := os.WriteFile(statusFile, []byte(initialContent), 0644); err != nil {
			t.Fatalf("failed to create temp file: %v", err)
		}

		// dev ticket default
		_, err := executeCommand("dev", "ticket", statusFile)
		if err != nil {
			t.Errorf("dev ticket default failed: %v", err)
		}

		// dev ticket list
		_, err = executeCommand("dev", "ticket", "list", statusFile)
		if err != nil {
			t.Errorf("dev ticket list failed: %v", err)
		}

		// dev ticket set
		_, err = executeCommand("dev", "ticket", "set", "DEN-001", "DONE", statusFile)
		if err != nil {
			t.Errorf("dev ticket set failed: %v", err)
		}

		content, _ := os.ReadFile(statusFile)
		if !strings.Contains(string(content), "[x]") {
			t.Errorf("expected ticket to be marked done, got: %s", string(content))
		}

		// Non-existent file error handling
		_, _ = executeCommand("dev", "ticket", "nonexistent.md")
		_, _ = executeCommand("dev", "ticket", "set", "DEN-999", "DONE", "nonexistent.md")
	})
}

func TestJetBrainsCommands(t *testing.T) {
	t.Run("jetbrains probe", func(t *testing.T) {
		_, err := executeCommand("jetbrains", "probe")
		if err != nil {
			t.Errorf("jetbrains probe failed: %v", err)
		}
	})

	t.Run("jetbrains open", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "test.go")
		_ = os.WriteFile(tmpFile, []byte("package main\n"), 0644)
		_, err := executeCommand("jetbrains", "open", tmpFile, "--line", "5")
		if err != nil {
			t.Errorf("jetbrains open failed: %v", err)
		}
	})

	t.Run("jetbrains problems", func(t *testing.T) {
		_, err := executeCommand("jetbrains", "problems", "main.go")
		if err != nil {
			t.Errorf("jetbrains problems failed: %v", err)
		}
	})

	t.Run("jetbrains symbols", func(t *testing.T) {
		_, err := executeCommand("jetbrains", "symbols", "main")
		if err != nil {
			t.Errorf("jetbrains symbols with query failed: %v", err)
		}

		_, err = executeCommand("jetbrains", "symbols")
		if err != nil {
			t.Errorf("jetbrains symbols without query failed: %v", err)
		}
	})

	t.Run("jetbrains build", func(t *testing.T) {
		_, err := executeCommand("jetbrains", "build")
		if err != nil {
			t.Errorf("jetbrains build failed: %v", err)
		}
	})

	t.Run("jetbrains run", func(t *testing.T) {
		_, err := executeCommand("jetbrains", "run", "DendriteBuild")
		if err != nil {
			t.Errorf("jetbrains run failed: %v", err)
		}
	})

	t.Run("jetbrains client provider error", func(t *testing.T) {
		orig := jetBrainsClientProvider
		defer func() { jetBrainsClientProvider = orig }()

		jetBrainsClientProvider = func() (*jetbrains.Client, error) {
			return nil, errors.New("simulated client initialization error")
		}

		_, _ = executeCommand("jetbrains", "probe")
		_, _ = executeCommand("jetbrains", "open", "main.go")
		_, _ = executeCommand("jetbrains", "problems", "main.go")
		_, _ = executeCommand("jetbrains", "symbols")
		_, _ = executeCommand("jetbrains", "build")
		_, _ = executeCommand("jetbrains", "run", "Default")
	})

	t.Run("jetbrains success paths with live server", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req jetbrains.JSONRPCRequest
			_ = json.NewDecoder(r.Body).Decode(&req)

			var res jetbrains.JSONRPCResponse
			res.JSONRPC = "2.0"
			res.ID = req.ID

			switch req.Method {
			case "initialize":
				res.Result = json.RawMessage(`{
					"protocolVersion": "2024-11-05",
					"serverInfo": {"name": "GoLand", "version": "2024.1"},
					"capabilities": {"tools": {}}
				}`)
			case "notifications/initialized":
				w.WriteHeader(http.StatusOK)
				return
			case "tools/list":
				res.Result = json.RawMessage(`{
					"tools": [
						{"name": "open_file_in_editor", "description": "Open file"},
						{"name": "get_file_problems", "description": "Get file problems"},
						{"name": "search_symbol", "description": "Search symbol in index"},
						{"name": "build_project", "description": "Build project"},
						{"name": "execute_run_configuration", "description": "Execute run config"}
					]
				}`)
			case "tools/call":
				paramsMap, _ := req.Params.(map[string]interface{})
				toolName, _ := paramsMap["name"].(string)
				switch toolName {
				case "open_file_in_editor":
					res.Result = json.RawMessage(`{"content":[{"type":"text","text":"opened"}]}`)
				case "get_file_problems":
					res.Result = json.RawMessage(`{"content":[{"type":"text","text":"[{\"severity\":\"WARNING\",\"line\":1,\"column\":1,\"description\":\"unused import\"}]"}]}`)
				case "search_symbol":
					res.Result = json.RawMessage(`{"content":[{"type":"text","text":"[{\"name\":\"main\",\"kind\":\"function\",\"file\":\"main.go\",\"line\":20}]"}]}`)
				case "build_project":
					res.Result = json.RawMessage(`{"content":[{"type":"text","text":"build succeeded in 1.2s"}]}`)
				case "execute_run_configuration":
					res.Result = json.RawMessage(`{"content":[{"type":"text","text":"process finished with exit code 0"}]}`)
				default:
					res.Result = json.RawMessage(`{"content":[{"type":"text","text":"ok"}]}`)
				}
			default:
				res.Result = json.RawMessage(`{}`)
			}

			_ = json.NewEncoder(w).Encode(res)
		}))
		defer server.Close()

		orig := jetBrainsClientProvider
		defer func() { jetBrainsClientProvider = orig }()

		jetBrainsClientProvider = func() (*jetbrains.Client, error) {
			transport := jetbrains.NewHTTPTransport(server.URL, server.Client())
			client, err := jetbrains.NewClient(transport)
			if err != nil {
				return nil, err
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if _, err := client.Connect(ctx); err != nil {
				return nil, err
			}
			return client, nil
		}

		tmpFile := filepath.Join(t.TempDir(), "dummy.go")
		_ = os.WriteFile(tmpFile, []byte("package main\n"), 0644)

		_, _ = executeCommand("jetbrains", "probe")
		_, _ = executeCommand("jetbrains", "open", tmpFile, "--line", "10")
		_, _ = executeCommand("jetbrains", "problems", tmpFile)
		_, _ = executeCommand("jetbrains", "symbols", "main")
		_, _ = executeCommand("jetbrains", "symbols")
		_, _ = executeCommand("jetbrains", "build")
		_, _ = executeCommand("jetbrains", "run", "DendriteBuild")
	})
}

func TestRunMain(t *testing.T) {
	// Env/dev commands render stacks/ and status files relative to the CWD; never touch the repository.
	t.Chdir(t.TempDir())
	err := runMain(context.Background(), []string{"version"})
	if err != nil {
		t.Errorf("runMain version failed: %v", err)
	}
}
