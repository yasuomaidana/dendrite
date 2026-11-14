package cli_test

import (
	"testing"

	"github.com/spf13/cobra"

	"Dendrite/cmd/dendrite/internal/cli"
	"Dendrite/cmd/dendrite/internal/clitest"
)

func TestMain(m *testing.M) { clitest.Main(m) }

func TestGatewayURL(t *testing.T) {
	cli.Globals = cli.GlobalOptions{}
	t.Setenv("LITELLM_PROXY_URL", "")
	if cli.GatewayV1URL() != "http://localhost:4000/v1" {
		t.Fatalf("default gateway = %s", cli.GatewayV1URL())
	}
	t.Setenv("LITELLM_PROXY_URL", "http://gw:4000/v1/")
	if cli.GatewayBaseURL() != "http://gw:4000" {
		t.Fatalf("env gateway base = %s", cli.GatewayBaseURL())
	}
	cli.Globals.GatewayURL = "http://flag:1"
	if cli.GatewayV1URL() != "http://flag:1/v1" {
		t.Fatalf("flag gateway = %s", cli.GatewayV1URL())
	}
}

func TestSharedFlagHelpers(t *testing.T) {
	var dry bool
	var ticket, model, project, file string
	c := &cobra.Command{Use: "x", RunE: func(*cobra.Command, []string) error { return nil }}
	cli.AddDryRunFlag(c, &dry)
	cli.AddTicketFlag(c, &ticket, "")
	cli.AddModelFlag(c, &model, "m")
	cli.AddProjectFlag(c, &project, "", "project")
	cli.AddFileFlag(c, &file, "", "file")
	if err := c.ParseFlags([]string{"--dry-run", "-t", "DEN-1", "-f", "a.md", "--project", "p"}); err != nil {
		t.Fatal(err)
	}
	if !dry || ticket != "DEN-1" || model != "m" || project != "p" || file != "a.md" {
		t.Fatalf("parsed %v %q %q %q %q", dry, ticket, model, project, file)
	}
	for _, v := range []cobra.PositionalArgs{cli.ExactArgs(1), cli.MinArgs(2), cli.MaxArgs(0), cli.RangeArgs(2, 3)} {
		if err := v(c, []string{"a"}); err != nil && !cli.IsUsageError(err) {
			t.Fatalf("validator error is not a usage error: %v", err)
		}
	}
}
