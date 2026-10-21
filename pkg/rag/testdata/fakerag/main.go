// Command fakerag answers the dendrite-rag sidecar methods used by rag.Retriever
// (health, index, search) with canned results, and records the params it received in
// the file named by FAKERAG_LOG. FAKERAG_FAIL=search makes search return an error.
package main

import (
	"bufio"
	"encoding/json"
	"os"
)

type request struct {
	ID     int64           `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	out := json.NewEncoder(os.Stdout)
	for in.Scan() {
		var req request
		if err := json.Unmarshal(in.Bytes(), &req); err != nil {
			continue
		}
		if log := os.Getenv("FAKERAG_LOG"); log != "" && req.Method != "health" {
			f, _ := os.OpenFile(log, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
			_, _ = f.WriteString(req.Method + " " + string(req.Params) + "\n")
			_ = f.Close()
		}
		resp := map[string]interface{}{"jsonrpc": "2.0", "id": req.ID}
		switch {
		case req.Method == "health":
			resp["result"] = map[string]string{"name": "dendrite-rag", "version": "0.1.0", "protocol": "1.0.0"}
		case req.Method == "index":
			resp["result"] = map[string]int{"files_scanned": 2, "files_changed": 1, "embed_pending": 7}
		case req.Method == "search" && os.Getenv("FAKERAG_FAIL") == "search":
			resp["error"] = map[string]interface{}{"code": -32000, "message": "boom", "data": map[string]string{"kind": "index"}}
		case req.Method == "search":
			resp["result"] = map[string]interface{}{"hits": []map[string]interface{}{{
				"chunk_id": "c1", "path": "pkg/a/a.go", "start_line": 3, "end_line": 9,
				"scope": []string{"impl Engine", "method search"}, "score": 0.03, "content": "func A() {}",
			}}}
		default:
			resp["error"] = map[string]interface{}{"code": -32601, "message": "method not found"}
		}
		_ = out.Encode(resp)
	}
}
