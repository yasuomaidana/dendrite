package main

import (
	"fmt"
	"io"
	"strings"

	"Dendrite/pkg/query/reuse"

	"github.com/spf13/cobra"
)

// newRefManager opens the Tier 1 reference registry on use, so --state-dir is honored.
func newRefManager() *reuse.RefManager { return reuse.NewRefManager("") }

// newAfferentCmd builds `dendrite afferent` (alias ref): Tier 1 source references for reuse and cleanroom synthesis.
func newAfferentCmd() *cobra.Command {
	cmd := newGroupCmd("afferent", "Manage Tier 1 source references for library reuse and cleanroom synthesis", "arbor",
		"ref", "dendritic-afferent", "aff")
	cmd.Example = "dendrite afferent list --json\n" +
		"dendrite afferent add --id totp-go --repo https://github.com/pquerna/otp --lang go --caps totp,hotp\n" +
		"dendrite afferent remove totp-go"

	list := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List registered source references",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAfferentList(newRefManager(), cmd.OutOrStdout())
		},
	}

	var ref reuse.SourceReference
	add := &cobra.Command{
		Use:   "add",
		Short: "Register a source reference",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if ref.Name == "" {
				ref.Name = ref.ID
			}
			if err := newRefManager().AddRef(ref); err != nil {
				return fmt.Errorf("register reference: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "[+] Registered Tier 1 Source Reference: %s (%s)\n", ref.Name, ref.Repository)
			return nil
		},
	}
	f := add.Flags()
	f.StringVar(&ref.ID, "id", "", "Reference ID")
	f.StringVar(&ref.Name, "name", "", "Display name (default: the ID)")
	f.StringVar(&ref.Repository, "repo", "", "Repository URL")
	f.StringVar(&ref.Language, "lang", "", "Primary language")
	f.StringSliceVar(&ref.Capabilities, "caps", nil, "Capabilities (comma-separated)")
	f.StringVar(&ref.License, "license", "", "SPDX license identifier")
	requireFlags(add, "id", "repo")

	remove := &cobra.Command{
		Use:     "remove <id>",
		Aliases: []string{"rm"},
		Short:   "Remove a source reference",
		Args:    exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := newRefManager().RemoveRef(args[0]); err != nil {
				return fmt.Errorf("remove reference %s: %w", args[0], err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "[+] Removed Tier 1 Source Reference: %s\n", args[0])
			return nil
		},
	}

	cmd.AddCommand(list, add, remove)
	return cmd
}

func runAfferentList(mgr *reuse.RefManager, out io.Writer) error {
	refs, err := mgr.ListRefs()
	if err != nil {
		return fmt.Errorf("list source references: %w", err)
	}
	if globals.JSON {
		if refs == nil {
			refs = []reuse.SourceReference{}
		}
		return writeJSON(out, refs)
	}
	fmt.Fprintf(out, "[+] Registered Source References (Tier 1: %d repos):\n", len(refs))
	for _, r := range refs {
		fmt.Fprintf(out, "  • %-18s [%-10s] %s\n", r.ID, r.Language, r.Repository)
		if len(r.Capabilities) > 0 {
			fmt.Fprintf(out, "    Capabilities: %s\n", strings.Join(r.Capabilities, ", "))
		}
	}
	return nil
}
