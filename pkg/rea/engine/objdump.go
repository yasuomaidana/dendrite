package engine

import (
	"bufio"
	"bytes"
	"context"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	objdumpLabel = regexp.MustCompile(`^([0-9a-fA-F]+) <(.+)>:$`)
	objdumpInsn  = regexp.MustCompile(`^\s+([0-9a-fA-F]+):\s*(.*)$`)
	objdumpCall  = regexp.MustCompile(`\bcall[a-z]*\s+[0-9a-fA-F]+\s+<([^>]+)>`)
)

// maxDisasmLines bounds the disassembly kept per function.
const maxDisasmLines = 80

func sortBySize(fns []Function) {
	sort.SliceStable(fns, func(i, j int) bool { return fns[i].Size > fns[j].Size })
}

// parseObjdump reads `objdump -d` output: one function per `<name>:` label, its size from the first and last
// instruction addresses, the functions it calls, and the start of its disassembly.
func parseObjdump(out []byte) []Function {
	var fns []Function
	var cur *Function
	var first, last uint64
	var lines int
	finish := func() {
		if cur != nil {
			cur.Size = last - first + 1
			cur.Calls = distinct(cur.Calls)
			fns = append(fns, *cur)
		}
	}
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 0, 64<<10), maxOutput)
	for sc.Scan() {
		line := sc.Text()
		if m := objdumpLabel.FindStringSubmatch(line); m != nil {
			finish()
			addr, _ := strconv.ParseUint(m[1], 16, 64)
			cur, first, last, lines = &Function{Name: m[2], Address: hex(addr)}, addr, addr, 0
			continue
		}
		if m := objdumpInsn.FindStringSubmatch(line); m != nil && cur != nil {
			addr, _ := strconv.ParseUint(m[1], 16, 64)
			last = addr
			if lines < maxDisasmLines {
				cur.Disassembly += hex(addr) + "  " + strings.TrimSpace(m[2]) + "\n"
				lines++
			}
			if c := objdumpCall.FindStringSubmatch(m[2]); c != nil {
				cur.Calls = append(cur.Calls, c[1])
			}
		}
	}
	finish()
	return fns
}

func distinct(items []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, it := range items {
		if !seen[it] {
			seen[it] = true
			out = append(out, it)
		}
	}
	return out
}

// runObjdump disassembles the target with objdump (or llvm-objdump) and reads the functions off its labels.
func (s *Service) runObjdump(ctx context.Context, c *Choice, target string) ([]Function, error) {
	out, err := s.run(ctx, c, target, []string{c.Spec.Name, "-d", "--no-show-raw-insn", "-C", TargetPath})
	if err != nil {
		return nil, err
	}
	return parseObjdump(out), nil
}
