// Command testsidecar is a minimal sidecar speaking the Dendrite sidecar protocol
// (newline-delimited JSON-RPC 2.0 over stdio), built by the sidecar package tests.
//
// TESTSIDECAR_PROTOCOL sets the protocol reported by health (default 1.0.0).
// TESTSIDECAR_MODE alters behaviour after the handshake: "badjson" answers with
// invalid JSON, "wrongid" answers with a mismatched id, "hang" never answers,
// "exit" exits without answering, "linger" keeps running after stdin closes.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
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
			case "wrongid":
				req.ID += 100
			case "hang":
				time.Sleep(time.Hour)
			case "exit":
				os.Exit(3)
			}
		}
		resp := map[string]interface{}{"jsonrpc": "2.0", "id": req.ID}
		switch req.Method {
		case "health":
			resp["result"] = map[string]string{"name": "testsidecar", "version": "0.1.0", "protocol": protocol}
		case "echo":
			resp["result"] = req.Params
		case "noresult":
		default:
			resp["error"] = map[string]interface{}{"code": -32601, "message": "method not found: " + req.Method}
		}
		_ = out.Encode(resp)
	}
	if mode == "linger" {
		time.Sleep(time.Hour)
	}
}
