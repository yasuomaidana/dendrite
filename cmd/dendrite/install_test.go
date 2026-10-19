package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"Dendrite/pkg/synapse"
)

func TestParseInstallArgs(t *testing.T) {
	o, err := parseInstallArgs([]string{"--dev", "--root", "/r", "--ref", "rag-rs=feature", "--ref=embedag=", "--target=/bin/d", "--no-build"})
	if err != nil {
		t.Fatal(err)
	}
	if o.channel != synapse.ChannelDev || o.root != "/r" || o.target != "/bin/d" || !o.noBuild {
		t.Fatalf("unexpected options %+v", o)
	}
	if o.refs["rag-rs"] != "feature" {
		t.Fatalf("rag-rs override = %q", o.refs["rag-rs"])
	}
	if ref, ok := o.refs["embedag"]; !ok || ref != "" {
		t.Fatalf("an empty --ref must clear the override, got %q (present %v)", ref, ok)
	}

	o, err = parseInstallArgs([]string{"--stable"})
	if err != nil || o.channel != synapse.ChannelStable {
		t.Fatalf("--stable: %+v %v", o, err)
	}
	o, err = parseInstallArgs(nil)
	if err != nil || o.channel != "" {
		t.Fatalf("no flag leaves the channel to the stored manifest: %+v %v", o, err)
	}

	for _, bad := range [][]string{{"--ref", "noequals"}, {"--ref", "=x"}, {"--root"}, {"--bogus"}} {
		if _, err := parseInstallArgs(bad); err == nil {
			t.Fatalf("expected an error for %v", bad)
		}
	}
}

func TestInstallCommand_Help(t *testing.T) {
	var out bytes.Buffer
	if err := runInstallCommand(context.Background(), []string{"--help"}, &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--dev", "main", "develop", "--ref <alias>=<ref>"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("usage missing %q:\n%s", want, out.String())
		}
	}
	if err := runInstallCommand(context.Background(), []string{"--bogus"}, &out); err == nil {
		t.Fatal("expected an unknown flag error")
	}
}

func TestSynapseCommand_Channel(t *testing.T) {
	t.Setenv("DENDRITE_HOME", t.TempDir())
	control, hub := t.TempDir(), t.TempDir()
	run := func(args ...string) (string, error) {
		var out bytes.Buffer
		err := runSynapseCommand(context.Background(), args, &out)
		return out.String(), err
	}
	if _, err := run("create", "stack", "--members", control+"=control:consumer,"+hub+"=mcphub:producer"); err != nil {
		t.Fatal(err)
	}
	if out, err := run("channel", "stack"); err != nil || !strings.Contains(out, "has no channel") {
		t.Fatalf("no channel yet: %q %v", out, err)
	}
	if _, err := run("set-source", "stack", "mcphub", "local", "--ref", "feature"); err != nil {
		t.Fatal(err)
	}
	out, err := run("channel", "stack", "dev")
	if err != nil || !strings.Contains(out, "channel: dev (tracks develop)") || !strings.Contains(out, "mcphub overrides it with feature") {
		t.Fatalf("set dev: %q %v", out, err)
	}
	st, _ := synapse.DefaultStore()
	s, _ := st.Load("stack")
	if s.Channel != synapse.ChannelDev {
		t.Fatalf("stored channel %q", s.Channel)
	}
	if _, err := run("channel", "stack", "nightly"); err == nil {
		t.Fatal("expected an invalid channel error")
	}
	if _, err := run("channel"); err == nil {
		t.Fatal("expected a usage error")
	}
	if _, err := run("channel", "missing", "dev"); err == nil {
		t.Fatal("expected a missing synapse error")
	}
}
