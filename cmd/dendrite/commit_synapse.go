package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"Dendrite/pkg/devtool"
	"Dendrite/pkg/projectid"
	"Dendrite/pkg/synapse"
	"Dendrite/pkg/synapsesync"
)

// peerConfirm decides whether a dirty synapse peer should be committed.
type peerConfirm func(alias string, dirty int) bool

// linePrompter asks "[y/N/a]" per peer; answering "a" approves every remaining peer.
func linePrompter(in io.Reader, out io.Writer) peerConfirm {
	reader := bufio.NewReader(in)
	all := false
	return func(alias string, dirty int) bool {
		if all {
			return true
		}
		fmt.Fprintf(out, "[?] Commit synapse peer %s (%d changed path(s))? [y/N/a] ", alias, dirty)
		line, _ := reader.ReadString('\n')
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "yes":
			return true
		case "a", "all":
			all = true
			return true
		}
		return false
	}
}

func stdinIsTerminal() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// commitSynapsePeers walks every synapse peer of startDir after the main commit: dirty peers are
// committed through the same AI-assisted engine when confirm approves (confirm nil = report only),
// and clean peers that are ahead of their upstream are pushed. Per-repo intent (ticket, message,
// file list, amend) applies only to the current project and is not carried over to peers.
func commitSynapsePeers(ctx context.Context, out io.Writer, startDir, synapseID string, base devtool.CommitOptions, confirm peerConfirm) error {
	_, targets, err := synapsesync.Discover(synapsesync.Options{StartDir: startDir, SynapseID: synapseID})
	if err != nil {
		return err
	}
	startID, _ := projectid.Read(startDir)

	peers := 0
	attention := 0
	for _, t := range targets {
		m := t.Member
		if t.Synapse == "" || (startID != "" && m.ProjectID == startID) || filepath.Clean(m.PathHint) == filepath.Clean(startDir) {
			continue
		}
		peers++
		if status := synapse.StatusOf(m); status != synapse.StatusOK {
			fmt.Fprintf(out, "[-] %s: member %s; run `dendrite synapse heal`\n", m.Alias, status)
			attention++
			continue
		}
		repo, err := devtool.InspectRepo(ctx, m.PathHint, false)
		if err != nil {
			fmt.Fprintf(out, "[-] %s: %v\n", m.Alias, err)
			attention++
			continue
		}

		switch {
		case len(repo.Dirty) > 0:
			if confirm == nil {
				fmt.Fprintf(out, "[!] %s has %d uncommitted path(s); re-run interactively or with --yes to commit\n", m.Alias, len(repo.Dirty))
				attention++
				continue
			}
			if !confirm(m.Alias, len(repo.Dirty)) {
				fmt.Fprintf(out, "[i] %s: skipped\n", m.Alias)
				continue
			}
			peerOpts := base
			peerOpts.RepoDir = m.PathHint
			peerOpts.TicketID, peerOpts.Message, peerOpts.Files, peerOpts.Amend, peerOpts.SmartTicket = "", "", nil, false, nil
			// Peers are committed as leftovers without a ticket of their own; the lifecycle guard
			// applies to the commit the user made, not to these.
			peerOpts.NoTicketGuard = true
			fmt.Fprintf(out, "[+] Committing synapse peer %s (%s)...\n", m.Alias, m.PathHint)
			res, err := devtool.ExecuteCommit(ctx, peerOpts)
			if err != nil {
				fmt.Fprintf(out, "[-] %s: commit failed: %v\n", m.Alias, err)
				attention++
				continue
			}
			if printCommitResult(out, res) {
				attention++
			}
		case repo.Ahead > 0 && repo.Behind == 0 && !base.NoPush && !base.DryRun:
			push := devtool.PushCurrentBranch(ctx, m.PathHint)
			switch {
			case push.Pushed:
				fmt.Fprintf(out, "[✓] %s: pushed %d commit(s) to %s/%s\n", m.Alias, repo.Ahead, push.Remote, push.Branch)
			case push.Err != nil:
				fmt.Fprintf(out, "[-] %s: push failed: %v\n", m.Alias, push.Err)
				attention++
			default:
				fmt.Fprintf(out, "[i] %s: push skipped (%s)\n", m.Alias, push.Reason)
			}
		case repo.Ahead > 0:
			fmt.Fprintf(out, "[i] %s: %d unpushed commit(s), %d behind upstream\n", m.Alias, repo.Ahead, repo.Behind)
		default:
			fmt.Fprintf(out, "[✓] %s: clean\n", m.Alias)
		}
	}

	if peers == 0 {
		fmt.Fprintln(out, "[i] No synapse peers found for this project")
	}
	if attention > 0 {
		return fmt.Errorf("%d synapse peer(s) need attention", attention)
	}
	return nil
}
