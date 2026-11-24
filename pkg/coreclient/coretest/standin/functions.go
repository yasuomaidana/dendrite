package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// functionPage is the most functions the stand-in puts in one page, like dendrite-core.
const functionPage = 48

// binFunctions answers bin.functions for a file that starts with "MZ". Its functions depend on the file's text, so
// a test chooses them: always main (5) and helper (15); "HARD" adds crypt_loop (22); "PARTIAL" adds walked_out (4, partial: its budget ran out); "MANY" adds 100 small ones
// (cyclomatic 3) so a list needs three pages; "BUDGET" sets truncated. Mode "nofunctions" plays a dendrite-core
// that predates the method, "funcfail" one that fails for another reason.
func binFunctions(params json.RawMessage, o options) (any, map[string]interface{}) {
	switch o.mode {
	case "nofunctions", "noinspect", "old", "pefail":
		return nil, rpcError(-32601, "method not found: bin.functions")
	case "funcfail":
		return nil, rpcError(-32000, "function analysis failed")
	}
	var p struct {
		Path          string `json:"path"`
		Offset        int    `json:"offset"`
		MinCyclomatic int    `json:"min_cyclomatic"`
	}
	_ = json.Unmarshal(params, &p)
	data, err := os.ReadFile(p.Path)
	if err != nil || len(data) < 2 || string(data[:2]) != "MZ" {
		return nil, rpcError(-32602, "not a PE image: "+p.Path)
	}
	all := standinFunctions(string(data))
	var kept []map[string]interface{}
	for _, f := range all {
		if f["cyclomatic"].(int) >= p.MinCyclomatic {
			kept = append(kept, f)
		}
	}
	from := min(max(p.Offset, 0), len(kept))
	to := min(from+functionPage, len(kept))
	var next any
	if to < len(kept) {
		next = to
	}
	return map[string]interface{}{
		"format": "pe", "arch": "x86_64", "total": len(kept), "offset": from, "next_offset": next,
		"truncated": strings.Contains(string(data), "BUDGET"), "functions": kept[from:to],
	}, nil
}

func standinFunctions(text string) []map[string]interface{} {
	fn := func(rva int, name string, cyclomatic int) map[string]interface{} {
		var n any
		if name != "" {
			n = name
		}
		start := fmt.Sprintf("0x%x", rva)
		return map[string]interface{}{"start": start, "end": fmt.Sprintf("0x%x", rva+0x40), "size": 0x40, "name": n,
			"cyclomatic": cyclomatic, "address": fmt.Sprintf("0x%x", 0x140000000+rva), "rva": start, "blocks": cyclomatic + 1, "instructions": 10 * cyclomatic}
	}
	out := []map[string]interface{}{fn(0x1000, "main", 5), fn(0x1100, "helper", 15)}
	if strings.Contains(text, "HARD") {
		out = append(out, fn(0x1200, "crypt_loop", 22))
	}
	if strings.Contains(text, "PARTIAL") {
		walked := fn(0x1300, "walked_out", 4)
		walked["partial"] = true
		out = append(out, walked)
	}
	if strings.Contains(text, "MANY") {
		for i := 0; i < 100; i++ {
			out = append(out, fn(0x2000+i*0x40, "", 3))
		}
	}
	return out
}
