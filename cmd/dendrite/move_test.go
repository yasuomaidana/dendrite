package main

import (
	"strings"
	"testing"
)

func TestTranslocate(t *testing.T) {
	if out, code := cliOutput(t, "translocate", "plan", "--ram", "8GB", "--disk", "2TB"); code != ExitOK || !strings.Contains(out, "SERVICE CAPACITY EVALUATION") {
		t.Fatalf("plan: exit %d\n%s", code, out)
	}
	if out, code := cliOutput(t, "move", "plan", "--json"); code != ExitOK || !strings.Contains(out, "{") {
		t.Fatalf("plan --json: exit %d\n%s", code, out)
	}
	for _, args := range [][]string{
		{"translocate", "plan", "--ram", "lots"},
		{"translocate", "plan", "--disk", "0GB"},
		{"translocate", "run", "plane"},
		{"translocate", "run", "--ssh", "host"},
		{"translocate", "run", "plane", "--ssh", "host", "--ram", "x"},
		{"translocate", "run", "plane", "--ssh", "host", "--timeout", "120"},
		{"translocate", "plane"},
	} {
		if _, code := cliOutput(t, args...); code != ExitUsage {
			t.Errorf("%v: exit %d, want %d", args, code, ExitUsage)
		}
	}
	if out, code := cliOutput(t, "translocate", "run", "plane,litellm", "--ssh", "nobody@127.0.0.1", "--port", "1", "--dry-run", "--timeout", "1s"); code == ExitUsage || !strings.Contains(out, "Services:      plane, litellm") {
		t.Fatalf("run --dry-run: exit %d\n%s", code, out)
	}
}

func TestParseSize(t *testing.T) {
	cases := []struct {
		in   string
		mb   uint64
		ok   bool
		toGB bool
	}{
		{"4GB", 4096, true, false}, {"512mb", 512, true, false}, {"2048", 2048, true, false},
		{"1TB", 1000, true, true}, {"250GB", 250, true, true}, {"x", 0, false, false}, {"0", 0, false, true},
	}
	for _, c := range cases {
		parse := parseSizeToMB
		if c.toGB {
			parse = parseSizeToGB
		}
		if got, ok := parse(c.in); got != c.mb || ok != c.ok {
			t.Errorf("parse(%q) = %d, %v", c.in, got, ok)
		}
	}
}
