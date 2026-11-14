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

// options are the environment the stand-in was started with.
type options struct{ protocol, mode string }

// handler answers one method: a result, or the JSON-RPC error that replaces it.
type handler func(params json.RawMessage, o options) (result any, failure map[string]interface{})

var handlers = map[string]handler{
	"health":                 health,
	"ast.complexity":         astComplexity,
	"ast.complexity_batch":   astComplexityBatch,
	"ast.fold_signatures":    astFold,
	"ast.last_complete_node": astLastNode,
	"ast.changed_symbols":    astChanged,
	"tokens.count":           tokensCount,
	"hash.chunk_id":          hashChunkID,
}

func main() {
	o := options{protocol: os.Getenv("TESTSIDECAR_PROTOCOL"), mode: os.Getenv("TESTSIDECAR_MODE")}
	if o.protocol == "" {
		o.protocol = "1.0.0"
	}
	in := bufio.NewScanner(os.Stdin)
	// A batch carries many files on one line, far past the scanner's 64 KB default.
	in.Buffer(make([]byte, 0, 1<<20), 256<<20)
	out := json.NewEncoder(os.Stdout)
	for in.Scan() {
		var req request
		if err := json.Unmarshal(in.Bytes(), &req); err != nil || o.misbehaves(req.Method) {
			continue
		}
		_ = out.Encode(reply(req, o))
	}
}

// misbehaves plays the failure the mode asks for on every method but health; true means no answer is sent.
func (o options) misbehaves(method string) bool {
	if method == "health" {
		return false
	}
	switch o.mode {
	case "badjson":
		fmt.Println("{not json")
		return true
	case "hang":
		time.Sleep(time.Hour)
	case "exit":
		os.Exit(3)
	}
	return false
}

func reply(req request, o options) map[string]interface{} {
	resp := map[string]interface{}{"jsonrpc": "2.0", "id": req.ID}
	h, ok := handlers[req.Method]
	if !ok {
		resp["error"] = rpcError(-32601, "method not found: "+req.Method)
		return resp
	}
	if result, failure := h(req.Params, o); failure != nil {
		resp["error"] = failure
	} else {
		resp["result"] = result
	}
	return resp
}

func rpcError(code int, message string) map[string]interface{} {
	return map[string]interface{}{"code": code, "message": message}
}

func health(_ json.RawMessage, o options) (any, map[string]interface{}) {
	return map[string]string{"name": "dendrite-core", "version": "0.1.0", "protocol": o.protocol}, nil
}

func astComplexity(params json.RawMessage, _ options) (any, map[string]interface{}) {
	var p struct {
		Lang   string `json:"lang"`
		Source string `json:"source"`
	}
	_ = json.Unmarshal(params, &p)
	functions, failed := complexityOf(p.Source)
	if failed {
		return nil, rpcError(-32602, "syntax error in source")
	}
	return functions, nil
}

func astComplexityBatch(params json.RawMessage, o options) (any, map[string]interface{}) {
	if o.mode == "nobatch" {
		return nil, rpcError(-32601, "method not found: ast.complexity_batch")
	}
	var p struct {
		Files []struct {
			Lang   string `json:"lang"`
			Source string `json:"source"`
		} `json:"files"`
	}
	_ = json.Unmarshal(params, &p)
	results := make([]map[string]interface{}, 0, len(p.Files))
	for _, f := range p.Files {
		if functions, failed := complexityOf(f.Source); failed {
			results = append(results, map[string]interface{}{"error": "syntax error in source"})
		} else {
			results = append(results, map[string]interface{}{"functions": functions})
		}
	}
	if o.mode == "shortbatch" && len(results) > 0 {
		results = results[1:]
	}
	return map[string]interface{}{"results": results}, nil
}

func astFold(params json.RawMessage, _ options) (any, map[string]interface{}) {
	var p struct {
		Lang        string   `json:"lang"`
		Source      string   `json:"source"`
		KeepSymbols []string `json:"keep_symbols"`
	}
	_ = json.Unmarshal(params, &p)
	if p.Source == "FAIL" || strings.Contains(p.Source, "FORCE_FOLD_ERROR") {
		return nil, rpcError(-32600, "fold failed")
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
	return map[string]interface{}{
		"source":        foldedSource,
		"folded":        foldedSymbols,
		"tokens_before": tb,
		"tokens_after":  ta,
	}, nil
}

func astLastNode(params json.RawMessage, _ options) (any, map[string]interface{}) {
	var p struct {
		Lang   string `json:"lang"`
		Buffer string `json:"buffer"`
	}
	_ = json.Unmarshal(params, &p)
	switch {
	case p.Buffer == "incomplete" || strings.Contains(p.Buffer, "NO_COMPLETE_NODE"):
		failure := rpcError(-32000, "the buffer holds no complete node")
		failure["data"] = map[string]string{"kind": "no_complete_node"}
		return nil, failure
	case p.Buffer == "FAIL":
		return nil, rpcError(-32600, "operation failed")
	}
	return map[string]interface{}{"offset": 42}, nil
}

func astChanged(params json.RawMessage, _ options) (any, map[string]interface{}) {
	var p struct {
		Lang   string `json:"lang"`
		Source string `json:"source"`
		Hunks  []struct {
			StartLine int `json:"start_line"`
			EndLine   int `json:"end_line"`
		} `json:"hunks"`
	}
	_ = json.Unmarshal(params, &p)
	if p.Source == "FAIL" {
		return nil, rpcError(-32600, "changed symbols failed")
	}
	target := func(hunk, start, end int) map[string]interface{} {
		return map[string]interface{}{"hunk": hunk, "symbol": "TargetFunc", "kind": "func", "start_line": start, "end_line": end}
	}
	var results []map[string]interface{}
	for i, h := range p.Hunks {
		results = append(results, target(i, h.StartLine, h.EndLine))
	}
	if len(results) == 0 {
		results = append(results, target(0, 1, 5))
	}
	return results, nil
}

func tokensCount(params json.RawMessage, _ options) (any, map[string]interface{}) {
	var p struct {
		Model string `json:"model"`
		Text  string `json:"text"`
	}
	_ = json.Unmarshal(params, &p)
	if p.Text == "FAIL" {
		return nil, rpcError(-32600, "token count failed")
	}
	return map[string]interface{}{"count": (len(p.Text)*115 + 399) / 400, "exact": false}, nil
}

func hashChunkID(params json.RawMessage, _ options) (any, map[string]interface{}) {
	var p struct {
		Path    string `json:"path"`
		Symbol  string `json:"symbol"`
		Content string `json:"content"`
	}
	_ = json.Unmarshal(params, &p)
	if p.Content == "FAIL" {
		return nil, rpcError(-32600, "chunk id failed")
	}
	sum := sha256.Sum256([]byte(p.Path + "\x00" + p.Symbol + "\x00" + p.Content))
	return map[string]interface{}{"hex": hex.EncodeToString(sum[:])}, nil
}

func contains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

// complexityOf answers ast.complexity for a source by the marker it contains; failed reports a source the
// sidecar would reject.
func complexityOf(source string) (functions []map[string]interface{}, failed bool) {
	fn := func(symbol string, end, cyclomatic, cognitive int) []map[string]interface{} {
		return []map[string]interface{}{{"symbol": symbol, "start_line": 1, "end_line": end, "cyclomatic": cyclomatic, "cognitive": cognitive}}
	}
	switch {
	case strings.Contains(source, "broken(") || strings.Contains(source, "FORCE_ERROR"):
		return nil, true
	case strings.Contains(source, "ExceededFunc") || strings.Contains(source, "FORCE_COMPLEXITY_HIGH"):
		return fn("ExceededFunc", 45, 11, 10), false
	case strings.Contains(source, "BoundaryFunc"):
		return fn("BoundaryFunc", 45, 10, 9), false
	case strings.Contains(source, "HighComplexity"):
		return fn("HighComplexity", 10, 15, 15), false
	case strings.Contains(source, "StripSpaces"):
		return fn("StripSpaces", 15, 4, 4), false
	case strings.Contains(source, "TestFunc"):
		return fn("TestFunc", 10, 3, 2), false
	}
	return fn("DefaultFunc", 5, 1, 0), false
}
