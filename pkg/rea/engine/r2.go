package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// r2Function is a function of rizin's or radare2's `aflj` answer.
type r2Function struct {
	Offset   uint64 `json:"offset"`
	Name     string `json:"name"`
	Size     uint64 `json:"size"`
	CC       int    `json:"cc"`
	CallRefs []struct {
		Addr uint64 `json:"addr"`
		Type string `json:"type"`
	} `json:"callrefs"`
}

// r2Disassembly is `pdfj` for one function.
type r2Disassembly struct {
	Addr uint64 `json:"addr"`
	Ops  []struct {
		Offset uint64 `json:"offset"`
		Disasm string `json:"disasm"`
	} `json:"ops"`
}

func hex(n uint64) string { return fmt.Sprintf("0x%x", n) }

// r2Argv builds the command of rizin or radare2 (they take the same one): no user configuration, run the commands
// and quit.
func r2Argv(bin, commands string) []string {
	return []string{bin, "-q", "-N", "-c", commands, TargetPath}
}

// parseR2Functions reads an `aflj` answer: the functions, with the addresses they call.
func parseR2Functions(out []byte) ([]Function, error) {
	var raw []r2Function
	if err := json.Unmarshal(bytes.TrimSpace(out), &raw); err != nil {
		return nil, fmt.Errorf("the engine did not answer a function list: %w", err)
	}
	fns := make([]Function, 0, len(raw))
	for _, r := range raw {
		f := Function{Name: r.Name, Address: hex(r.Offset), Size: r.Size, Cyclomatic: r.CC}
		for _, c := range r.CallRefs {
			if strings.EqualFold(c.Type, "CALL") {
				f.Calls = append(f.Calls, hex(c.Addr))
			}
		}
		fns = append(fns, f)
	}
	return fns, nil
}

// parseR2Disassembly reads the `pdfj` answers (one JSON value each) into text per function address.
func parseR2Disassembly(out []byte) map[string]string {
	text := map[string]string{}
	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		var d r2Disassembly
		if err := dec.Decode(&d); err != nil {
			if err != io.EOF {
				return text // a broken tail leaves what was read
			}
			return text
		}
		var sb strings.Builder
		for _, op := range d.Ops {
			fmt.Fprintf(&sb, "%s  %s\n", hex(op.Offset), op.Disasm)
		}
		text[hex(d.Addr)] = sb.String()
	}
}

// runR2 lists the functions with rizin or radare2, then reads the disassembly of the largest ones.
func (s *Service) runR2(ctx context.Context, c *Choice, target string) ([]Function, error) {
	bin := c.Spec.Name
	out, err := s.run(ctx, c, target, r2Argv(bin, "aaa;aflj"))
	if err != nil {
		return nil, err
	}
	fns, err := parseR2Functions(out)
	if err != nil {
		return nil, err
	}
	largest := append([]Function(nil), fns...)
	sortBySize(largest)
	var cmds []string
	for i, f := range largest {
		if i == detailLimit {
			break
		}
		cmds = append(cmds, "pdfj @ "+f.Address)
	}
	if len(cmds) == 0 {
		return fns, nil
	}
	detail, err := s.run(ctx, c, target, r2Argv(bin, "aaa;"+strings.Join(cmds, ";")))
	if err != nil {
		return nil, err
	}
	asm := parseR2Disassembly(detail)
	for i := range fns {
		fns[i].Disassembly = asm[fns[i].Address]
	}
	return fns, nil
}
