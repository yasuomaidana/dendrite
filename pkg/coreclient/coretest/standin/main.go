// Command testsidecar is a minimal sidecar speaking the Dendrite sidecar protocol
// (newline-delimited JSON-RPC 2.0 over stdio), implementing dendrite-core methods
// for testing pkg/coreclient.
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

type request struct {
	ID     int64           `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

func main() {
	protocol := os.Getenv("TESTSIDECAR_PROTOCOL")
	if protocol == "" {
		protocol = "1.0.0"
	}
	mode := os.Getenv("TESTSIDECAR_MODE")
	in := bufio.NewScanner(os.Stdin)
	out := json.NewEncoder(os.Stdout)
	for in.Scan() {
		var req request
		if err := json.Unmarshal(in.Bytes(), &req); err != nil {
			continue
		}
		if req.Method != "health" {
			switch mode {
			case "badjson":
				fmt.Println("{not json")
				continue
			case "hang":
				time.Sleep(time.Hour)
			case "exit":
				os.Exit(3)
			}
		}
		resp := map[string]interface{}{"jsonrpc": "2.0", "id": req.ID}
		switch req.Method {
		case "health":
			resp["result"] = map[string]string{"name": "dendrite-core", "version": "0.1.0", "protocol": protocol}
		case "ast.complexity":
			var p struct {
				Lang   string `json:"lang"`
				Source string `json:"source"`
			}
			_ = json.Unmarshal(req.Params, &p)
			if strings.Contains(p.Source, "broken(") || strings.Contains(p.Source, "FORCE_ERROR") {
				resp["error"] = map[string]interface{}{"code": -32602, "message": "syntax error in source"}
			} else if strings.Contains(p.Source, "ExceededFunc") || strings.Contains(p.Source, "FORCE_COMPLEXITY_HIGH") {
				resp["result"] = []map[string]interface{}{
					{"symbol": "ExceededFunc", "start_line": 1, "end_line": 45, "cyclomatic": 11, "cognitive": 10},
				}
			} else if strings.Contains(p.Source, "BoundaryFunc") {
				resp["result"] = []map[string]interface{}{
					{"symbol": "BoundaryFunc", "start_line": 1, "end_line": 45, "cyclomatic": 10, "cognitive": 9},
				}
			} else if strings.Contains(p.Source, "HighComplexity") {
				resp["result"] = []map[string]interface{}{
					{"symbol": "HighComplexity", "start_line": 1, "end_line": 10, "cyclomatic": 15, "cognitive": 15},
				}
			} else if strings.Contains(p.Source, "StripSpaces") {
				resp["result"] = []map[string]interface{}{
					{"symbol": "StripSpaces", "start_line": 1, "end_line": 15, "cyclomatic": 4, "cognitive": 4},
				}
			} else if strings.Contains(p.Source, "TestFunc") {
				resp["result"] = []map[string]interface{}{
					{"symbol": "TestFunc", "start_line": 1, "end_line": 10, "cyclomatic": 3, "cognitive": 2},
				}
			} else {
				resp["result"] = []map[string]interface{}{
					{"symbol": "DefaultFunc", "start_line": 1, "end_line": 5, "cyclomatic": 1, "cognitive": 0},
				}
			}
		case "ast.fold_signatures":
			var p struct {
				Lang        string   `json:"lang"`
				Source      string   `json:"source"`
				KeepSymbols []string `json:"keep_symbols"`
			}
			_ = json.Unmarshal(req.Params, &p)
			if p.Source == "FAIL" || strings.Contains(p.Source, "FORCE_FOLD_ERROR") {
				resp["error"] = map[string]interface{}{"code": -32600, "message": "fold failed"}
				break
			}
			foldedSource := p.Source
			var foldedSymbols []string
			if strings.Contains(p.Source, "CallAPI") && contains(p.KeepSymbols, "NewClient") {
				foldedSource = strings.ReplaceAll(foldedSource, "println(\"heavy logic\")\n\tfor i := 0; i < 3; i++ {\n\t\tprintln(\"retrying request against\", c.Endpoint, \"attempt\", i)\n\t\tif c.Endpoint == \"\" {\n\t\t\treturn nil\n\t\t}\n\t}\n\tprintln(\"request completed successfully after retries\")\n\treturn nil", "/* pruned */")
				foldedSymbols = append(foldedSymbols, "CallAPI")
			} else if strings.Contains(p.Source, "StandaloneHelper") && contains(p.KeepSymbols, "KeptHelper") {
				foldedSource = strings.ReplaceAll(foldedSource, "y := x * 2\n\treturn y", "/* pruned */")
				foldedSymbols = append(foldedSymbols, "StandaloneHelper")
			} else if strings.Contains(p.Source, "Step") && len(p.KeepSymbols) == 0 {
				foldedSource = strings.ReplaceAll(foldedSource, "println(\"stepping\")", "/* pruned */")
				foldedSymbols = append(foldedSymbols, "Step")
			} else if strings.Contains(p.Source, "Pruned") {
				foldedSource = strings.ReplaceAll(foldedSource, "println(\"prune me\")", "/* pruned */")
				foldedSymbols = append(foldedSymbols, "Pruned")
			}
			tb := (len(p.Source)*115 + 399) / 400
			ta := (len(foldedSource)*115 + 399) / 400
			resp["result"] = map[string]interface{}{
				"source":        foldedSource,
				"folded":        foldedSymbols,
				"tokens_before": tb,
				"tokens_after":  ta,
			}
		case "ast.last_complete_node":
			var p struct {
				Lang   string `json:"lang"`
				Buffer string `json:"buffer"`
			}
			_ = json.Unmarshal(req.Params, &p)
			if p.Buffer == "incomplete" || strings.Contains(p.Buffer, "NO_COMPLETE_NODE") {
				resp["error"] = map[string]interface{}{
					"code":    -32000,
					"message": "the buffer holds no complete node",
					"data":    map[string]string{"kind": "no_complete_node"},
				}
			} else if p.Buffer == "FAIL" {
				resp["error"] = map[string]interface{}{"code": -32600, "message": "operation failed"}
			} else {
				resp["result"] = map[string]interface{}{"offset": 42}
			}
		case "ast.changed_symbols":
			var p struct {
				Lang   string `json:"lang"`
				Source string `json:"source"`
				Hunks  []struct {
					StartLine int `json:"start_line"`
					EndLine   int `json:"end_line"`
				} `json:"hunks"`
			}
			_ = json.Unmarshal(req.Params, &p)
			if p.Source == "FAIL" {
				resp["error"] = map[string]interface{}{"code": -32600, "message": "changed symbols failed"}
				break
			}
			var results []map[string]interface{}
			for i, h := range p.Hunks {
				results = append(results, map[string]interface{}{
					"hunk":       i,
					"symbol":     "TargetFunc",
					"kind":       "func",
					"start_line": h.StartLine,
					"end_line":   h.EndLine,
				})
			}
			if len(results) == 0 {
				results = append(results, map[string]interface{}{
					"hunk":       0,
					"symbol":     "TargetFunc",
					"kind":       "func",
					"start_line": 1,
					"end_line":   5,
				})
			}
			resp["result"] = results
		case "tokens.count":
			var p struct {
				Model string `json:"model"`
				Text  string `json:"text"`
			}
			_ = json.Unmarshal(req.Params, &p)
			if p.Text == "FAIL" {
				resp["error"] = map[string]interface{}{"code": -32600, "message": "token count failed"}
				break
			}
			resp["result"] = map[string]interface{}{
				"count": (len(p.Text)*115 + 399) / 400,
				"exact": false,
			}
		case "hash.chunk_id":
			var p struct {
				Path    string `json:"path"`
				Symbol  string `json:"symbol"`
				Content string `json:"content"`
			}
			_ = json.Unmarshal(req.Params, &p)
			if p.Content == "FAIL" {
				resp["error"] = map[string]interface{}{"code": -32600, "message": "chunk id failed"}
				break
			}
			sum := sha256.Sum256([]byte(p.Path + "\x00" + p.Symbol + "\x00" + p.Content))
			resp["result"] = map[string]interface{}{"hex": hex.EncodeToString(sum[:])}
		default:
			resp["error"] = map[string]interface{}{"code": -32601, "message": "method not found: " + req.Method}
		}
		_ = out.Encode(resp)
	}
}

func contains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}
