package soma

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"Dendrite/cmd/dendrite/internal/cli"
	"Dendrite/pkg/repair"
	"Dendrite/pkg/synapse"
	"Dendrite/pkg/ticketlock"
)

// NewRepairCmd builds `dendrite repair`: temporary repairs of dependencies, tracked as chains.
func NewRepairCmd() *cobra.Command {
	cmd := cli.NewGroupCmd("repair", "Fix a dependency on a branch or fork of its own, tracked until the fix is merged and released", "soma")
	cmd.Long = "A repair points this project at a branch of one of its dependencies while a bug or a security hole is fixed\n" +
		"there, and remembers how the dependency was named before. Run `repair start` again inside the repair's\n" +
		"working copy to repair a dependency of the dependency: the repairs form a chain."
	cmd.Example = "dendrite repair start embedag --dep-ticket EMBEDAG-12\n" +
		"dendrite repair start serde --repo https://github.com/serde-rs/serde --fork https://github.com/me/serde"
	cmd.AddCommand(newRepairStartCmd())
	return cmd
}

func newRepairStartCmd() *cobra.Command {
	var o repair.StartOptions
	cmd := &cobra.Command{
		Use:   "start <dependency>",
		Short: "Get a working copy of a dependency on a repair branch and point this project at it",
		Long: "The repair branch is named after the ticket: <TICKET>-<slug>/main. A dependency that is a synapse peer is\n" +
			"repaired in its own repository, on a branch named after the peer's own ticket (--dep-ticket; file it with\n" +
			"`dendrite chronos request`). Any other dependency needs --repo, and --fork when you cannot push to it.",
		Args: cli.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o.Consumer, o.Dependency = ".", args[0]
			return runRepairStart(cmd.Context(), o, cmd.OutOrStdout())
		},
	}
	f := cmd.Flags()
	f.StringVar(&o.Ticket, "ticket", "", "This project's ticket (default: the ticket of the current branch)")
	f.StringVar(&o.DepTicket, "dep-ticket", "", "The dependency project's own ticket for the fix; names the repair branch")
	f.StringVar(&o.Upstream, "repo", "", "URL of the dependency's repository (default: the synapse peer's)")
	f.StringVar(&o.Fork, "fork", "", "URL of a fork to push the repair branch to")
	f.StringVar(&o.Checkout, "checkout", "", "Local checkout of the dependency to branch from (default: the synapse peer's)")
	return cmd
}

func runRepairStart(ctx context.Context, o repair.StartOptions, out io.Writer) error {
	if o.Ticket == "" {
		branch, _ := exec.CommandContext(ctx, "git", "-C", o.Consumer, "symbolic-ref", "--short", "HEAD").Output()
		ticket, ok := ticketlock.TicketOfBranch(strings.TrimSpace(string(branch)))
		if !ok {
			return cli.UsageErrorf("this branch is not named after a ticket: pass --ticket")
		}
		o.Ticket = strings.ToUpper(ticket)
	}
	if err := fillFromPeer(&o); err != nil {
		return err
	}
	ledger, err := repair.Open()
	if err != nil {
		return err
	}
	res, err := repair.Start(ctx, ledger, o)
	if err != nil {
		return err
	}
	if cli.Globals.JSON {
		return cli.WriteJSON(out, res)
	}
	l := res.Link
	fmt.Fprintf(out, "[✓] Repair %s started\n", l.ID)
	fmt.Fprintf(out, "    Working copy: %s\n", l.Copy)
	fmt.Fprintf(out, "    Branch:       %s  (pushed to %s)\n", l.Branch, l.Remote)
	for _, e := range l.Original {
		fmt.Fprintf(out, "    Was:          %s  (%s, %s)\n", e.Raw, e.Manifest, e.Mode)
	}
	if l.Parent != "" {
		fmt.Fprintf(out, "    Part of:      %s\n", l.Parent)
	}
	for _, w := range res.Warnings {
		fmt.Fprintf(out, "[!] %s\n", w)
	}
	fmt.Fprintf(out, "    cd %s\n", l.Copy)
	return nil
}

// fillFromPeer takes the checkout and the repository of a dependency that is a synapse peer. A peer is repaired
// under a ticket of its own project, so one must be named.
func fillFromPeer(o *repair.StartOptions) error {
	if o.Checkout != "" || o.Upstream != "" || o.Fork != "" {
		return nil
	}
	st, err := synapse.DefaultStore()
	if err != nil {
		return err
	}
	peer, ok := repair.FindPeer(st, o.Consumer, o.Dependency)
	if !ok {
		return cli.UsageErrorf("%s is not a synapse peer of this project: pass --repo <url> (and --fork <url> when you cannot push to it)", o.Dependency)
	}
	if o.DepTicket == "" {
		return cli.UsageErrorf("%s is the synapse peer %s: file the fix in its project and pass the ticket\n  dendrite chronos request %s --name \"...\" --desc \"goal and acceptance criteria\" --from %s\n  dendrite repair start %s --dep-ticket <its ticket>", o.Dependency, peer.Alias, peer.Alias, o.Ticket, o.Dependency)
	}
	o.Checkout, o.Upstream = peer.Checkout, peer.Remote
	if o.Checkout == "" && o.Upstream == "" {
		return cli.UsageErrorf("the synapse has no checkout and no remote for %s: pass --repo <url>", peer.Alias)
	}
	return nil
}
