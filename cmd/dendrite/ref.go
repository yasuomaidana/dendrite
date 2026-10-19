package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"Dendrite/pkg/query/reuse"
)

var defaultRefMgr = reuse.NewRefManager(".dendrite/refs.json")

// newAfferentCmd builds `dendrite afferent`: manage Tier 1 source references.
func newAfferentCmd() *cobra.Command {
	cmd := newGroupCmd(
		"afferent",
		"Manage Tier 1 source references for library reuse and cleanroom synthesis",
		"arbor",
		"ref", "dendritic-afferent", "aff",
	)
	cmd.Example = strings.Join([]string{
		"dendrite afferent list",
		"dendrite afferent add --id ref-gin --name gin --repo https://github.com/gin-gonic/gin --lang go --caps http,routing",
		"dendrite afferent remove ref-gin",
	}, "\n")

	cmd.AddCommand(
		newAfferentListCmd(),
		newAfferentAddCmd(),
		newAfferentRemoveCmd(),
	)
	return cmd
}

func newAfferentListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "List registered Tier 1 source references",
		Args:    noArgs,
		Example: "dendrite afferent list --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAfferentList(cmd.OutOrStdout())
		},
	}
}

func runAfferentList(out io.Writer) error {
	refs, err := defaultRefMgr.ListRefs()
	if err != nil {
		return fmt.Errorf("failed to list source references: %w", err)
	}

	if globals.JSON {
		data, err := json.MarshalIndent(refs, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(out, string(data))
		return nil
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

type afferentAddOptions struct {
	ID      string
	Name    string
	Repo    string
	Lang    string
	Caps    []string
	License string
}

func newAfferentAddCmd() *cobra.Command {
	var o afferentAddOptions
	cmd := &cobra.Command{
		Use:     "add",
		Short:   "Register a Tier 1 source reference",
		Args:    noArgs,
		Example: "dendrite afferent add --id ref-gin --name gin --repo https://github.com/gin-gonic/gin --lang go --caps http,routing --license MIT",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAfferentAdd(o, cmd.OutOrStdout())
		},
	}
	f := cmd.Flags()
	f.SetNormalizeFunc(func(fs *pflag.FlagSet, name string) pflag.NormalizedName {
		switch name {
		case "repository":
			name = "repo"
		case "language":
			name = "lang"
		case "capabilities":
			name = "caps"
		}
		return pflag.NormalizedName(name)
	})
	f.StringVar(&o.ID, "id", "", "Reference identifier (required)")
	f.StringVar(&o.Repo, "repo", "", "Repository URL (required)")
	f.StringVar(&o.Name, "name", "", "Display name (default: id)")
	f.StringVar(&o.Lang, "lang", "", "Primary language")
	f.StringSliceVar(&o.Caps, "caps", nil, "Comma-separated capabilities")
	f.StringVar(&o.License, "license", "", "SPDX license identifier")
	requireFlags(cmd, "id", "repo")
	return cmd
}

func runAfferentAdd(o afferentAddOptions, out io.Writer) error {
	var caps []string
	for _, c := range o.Caps {
		if trimmed := strings.TrimSpace(c); trimmed != "" {
			caps = append(caps, trimmed)
		}
	}
	name := o.Name
	if name == "" {
		name = o.ID
	}
	ref := reuse.SourceReference{
		ID:           o.ID,
		Name:         name,
		Repository:   o.Repo,
		Language:     o.Lang,
		Capabilities: caps,
		License:      o.License,
	}

	if err := defaultRefMgr.AddRef(ref); err != nil {
		return fmt.Errorf("failed to register reference: %w", err)
	}
	fmt.Fprintf(out, "[+] Registered Tier 1 Source Reference: %s (%s)\n", ref.Name, ref.Repository)
	return nil
}

func newAfferentRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "remove <id>",
		Aliases: []string{"rm"},
		Short:   "Remove a registered Tier 1 source reference",
		Args:    exactArgs(1),
		Example: "dendrite afferent remove ref-gin",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAfferentRemove(args[0], cmd.OutOrStdout())
		},
	}
}

func runAfferentRemove(id string, out io.Writer) error {
	if err := defaultRefMgr.RemoveRef(id); err != nil {
		return fmt.Errorf("failed to remove reference: %w", err)
	}
	fmt.Fprintf(out, "[+] Removed Tier 1 Source Reference: %s\n", id)
	return nil
}

func handleRefCommand(ctx context.Context, action string, args []string) {
	_ = invokeCmd(ctx, newAfferentCmd(), append([]string{action}, args...)...)
}

func printRefUsage() {
	_ = newAfferentCmd().Help()
}
