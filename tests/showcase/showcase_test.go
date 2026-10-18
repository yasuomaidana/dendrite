package showcase_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"Dendrite/pkg/mcpserver"
	"Dendrite/pkg/rea"
	"Dendrite/pkg/rea/showcase"
)

func TestShowcase_EndToEndVerification(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	report, err := showcase.RunShowcase(ctx, "all")
	if err != nil {
		t.Fatalf("RunShowcase('all') failed: %v", err)
	}

	if report.OverallStatus != "PASSED" {
		t.Errorf("expected overall status PASSED, got: %s", report.OverallStatus)
	}
	if report.TotalTargets != 5 {
		t.Errorf("expected 5 targets, got %d", report.TotalTargets)
	}
	if report.SucceededTargets != 5 {
		t.Errorf("expected 5 succeeded targets, got %d", report.SucceededTargets)
	}
	if report.FailedTargets != 0 {
		t.Errorf("expected 0 failed targets, got %d", report.FailedTargets)
	}
	if report.OverallParityPercent != 100.0 {
		t.Errorf("expected 100%% overall parity, got %.2f%%", report.OverallParityPercent)
	}

	// Invariant audit per target
	for _, res := range report.Results {
		t.Run(res.Target, func(t *testing.T) {
			if !res.PassedValidation {
				t.Errorf("target %s failed validation", res.Target)
			}
			if res.Parity.ParityPercent != 100.0 {
				t.Errorf("target %s parity expected 100%%, got %.2f%%", res.Target, res.Parity.ParityPercent)
			}
			if res.Parity.FailedVectors != 0 {
				t.Errorf("target %s has %d failed vectors", res.Target, res.Parity.FailedVectors)
			}

			// Token budget invariant (< 4,000 tokens)
			if res.Evidence == nil {
				t.Fatalf("target %s missing evidence bundle", res.Target)
			}
			if res.Evidence.TokenEstimate > rea.MaxEvidenceTokens {
				t.Errorf("target %s exceeded token cap: %d > %d", res.Target, res.Evidence.TokenEstimate, rea.MaxEvidenceTokens)
			}
			if res.Evidence.Truncated {
				t.Errorf("target %s unexpectedly truncated within budget", res.Target)
			}

			// Sandbox security invariant
			if !res.SandboxSecured {
				t.Errorf("target %s must be sandbox secured", res.Target)
			}

			// SPDX compliance invariant
			if !res.SPDX.Compliant || !res.SPDX.OSIApproved {
				t.Errorf("target %s failed SPDX open-source compliance: %+v", res.Target, res.SPDX)
			}

			// ADR document invariant
			if res.ADR == nil {
				t.Fatalf("target %s missing ADR document", res.Target)
			}
			if !strings.HasPrefix(res.ADR.ID, "ADR-REA-") {
				t.Errorf("target %s invalid ADR ID: %s", res.Target, res.ADR.ID)
			}
			if !strings.Contains(res.ADRMarkdown, "---") || !strings.Contains(res.ADRMarkdown, "id: "+res.ADR.ID) {
				t.Errorf("target %s missing YAML frontmatter in ADR", res.Target)
			}
		})
	}
}

func TestShowcase_IndividualTargets(t *testing.T) {
	targets := []string{"aegis", "dxball", "ctf", "th04", "mattermost"}
	ctx := context.Background()

	for _, target := range targets {
		t.Run(target, func(t *testing.T) {
			rep, err := showcase.RunShowcase(ctx, target)
			if err != nil {
				t.Fatalf("failed to run target %s: %v", target, err)
			}
			if rep.OverallStatus != "PASSED" || rep.TotalTargets != 1 || rep.SucceededTargets != 1 {
				t.Errorf("target %s unexpected report: %+v", target, rep)
			}
			if rep.Results[0].Parity.ParityPercent != 100.0 {
				t.Errorf("target %s parity not 100%%", target)
			}
		})
	}
}

func TestShowcase_CLIIntegration(t *testing.T) {
	// Locate bin/dendrite or compile if needed
	binPath, err := filepath.Abs("../../bin/dendrite")
	if err != nil {
		t.Fatalf("failed to resolve bin path: %v", err)
	}

	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		buildCmd := exec.Command("go", "build", "-o", binPath, "../../cmd/dendrite")
		if out, err := buildCmd.CombinedOutput(); err != nil {
			t.Fatalf("failed to compile dendrite binary: %v (%s)", err, string(out))
		}
	}

	// 1. Test rea list
	listCmd := exec.Command(binPath, "rea", "list")
	listOut, err := listCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dendrite rea list failed: %v (%s)", err, string(listOut))
	}
	if !strings.Contains(string(listOut), "Registered Open-Source REA Showcase Targets (5)") {
		t.Errorf("unexpected rea list output: %s", string(listOut))
	}

	// 2. Test rea showcase all
	allCmd := exec.Command(binPath, "rea", "showcase", "all")
	allOut, err := allCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dendrite rea showcase all failed: %v (%s)", err, string(allOut))
	}
	if !strings.Contains(string(allOut), "DENDRITE REA CLEANROOM VERIFICATION SCORECARD: PASSED") {
		t.Errorf("unexpected rea showcase all output: %s", string(allOut))
	}

	// 3. Test rea showcase markdown
	mdCmd := exec.Command(binPath, "rea", "showcase", "aegis", "--format", "markdown")
	mdOut, err := mdCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dendrite rea showcase markdown failed: %v (%s)", err, string(mdOut))
	}
	if !strings.Contains(string(mdOut), "# Open-Source REA Showcase & Cleanroom Parity Scorecard") {
		t.Errorf("unexpected markdown output: %s", string(mdOut))
	}

	// 4. Test rea showcase json
	jsonCmd := exec.Command(binPath, "rea", "showcase", "ctf", "--format", "json")
	jsonOut, err := jsonCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dendrite rea showcase json failed: %v (%s)", err, string(jsonOut))
	}
	if !strings.Contains(string(jsonOut), `"overall_status": "PASSED"`) {
		t.Errorf("unexpected json output: %s", string(jsonOut))
	}
}

func TestShowcase_MCPServerProtocol(t *testing.T) {
	srv := mcpserver.NewServer(mcpserver.ServerConfig{})
	mcpserver.RegisterDefaultTools(srv, mcpserver.Dependencies{DevStatus: "../../DEV_STATUS.md"})

	ctx := context.Background()

	req := mcpserver.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
	}
	req.Params, _ = json.Marshal(map[string]interface{}{
		"name": "dendrite_rea_showcase",
		"arguments": map[string]interface{}{
			"target": "mattermost",
			"format": "summary",
		},
	})

	reqBytes, _ := json.Marshal(req)
	var inBuf bytes.Buffer
	inBuf.Write(reqBytes)
	inBuf.WriteString("\n")

	var outBuf bytes.Buffer
	err := srv.ServeStdio(ctx, &inBuf, &outBuf)
	if err != nil {
		t.Fatalf("ServeStdio error: %v", err)
	}

	var resp mcpserver.JSONRPCResponse
	if err := json.Unmarshal(outBuf.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode JSON-RPC response: %v (%s)", err, outBuf.String())
	}

	if resp.Error != nil {
		t.Fatalf("unexpected JSON-RPC error: %+v", resp.Error)
	}

	resMap, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected Result map, got: %T", resp.Result)
	}
	contentList, ok := resMap["content"].([]interface{})
	if !ok || len(contentList) == 0 {
		t.Fatalf("expected content array in result, got: %+v", resMap)
	}
	textItem := contentList[0].(map[string]interface{})
	textVal := textItem["text"].(string)
	if !strings.Contains(textVal, "REA Showcase Verification (PASSED)") {
		t.Errorf("unexpected tool output: %s", textVal)
	}
}
