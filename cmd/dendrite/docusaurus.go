package main

import (
	"Dendrite/pkg/mcpserver"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"

	"Dendrite/pkg/docgen/reflect"
	"Dendrite/pkg/docusaurus"

	"github.com/spf13/cobra"
)

const defaultSiteDir = "website"

// newCodexCmd builds `dendrite codex` (alias doc, docs): reference docs, the Docusaurus site and Plane page sync.
func newCodexCmd() *cobra.Command {
	cmd := newGroupCmd("codex", "Docs-as-Code reflection, Docusaurus synthesis and portal", "cortex",
		"doc", "docs", "engram-codex")
	cmd.Example = "dendrite codex generate --reference\n" +
		"dendrite codex audit --strict\n" +
		"dendrite codex generate --out website --tier internal\n" +
		"dendrite codex site serve --port 3000\n" +
		"dendrite codex plane push --dir docs"
	cmd.AddCommand(newCodexGenerateCmd(), newCodexAuditCmd(), newCodexReflectCmd(), newCodexSiteCmd(), newCodexPlaneCmd())
	return cmd
}

type codexGenerateOptions struct {
	Reference bool
	Docs      string
	Out       string
	Title     string
	Port      int
	Tier      string
}

func newCodexGenerateCmd() *cobra.Command {
	var o codexGenerateOptions
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate the Docusaurus site, or with --reference the CLI and MCP reference pages",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if o.Reference {
				return runCodexReference(o.Docs, cmd.OutOrStdout())
			}
			return runCodexSite(cmd.Context(), docusaurus.NewService(), o, cmd.OutOrStdout())
		},
	}
	f := cmd.Flags()
	f.BoolVar(&o.Reference, "reference", false, "Write docs/cli and docs/mcp from the live command tree and MCP tools")
	f.StringVar(&o.Docs, "docs", "docs", "Documentation directory")
	f.StringVar(&o.Out, "out", defaultSiteDir, "Site output directory")
	f.StringVar(&o.Title, "title", "Dendrite Documentation", "Site title")
	f.IntVar(&o.Port, "port", 3000, "Preview port written to the site config")
	f.StringVar(&o.Tier, "tier", string(docusaurus.SharingTierFederated), "Sharing tier: public, internal, federated or private")
	return cmd
}

func runCodexReference(docsDir string, out io.Writer) error {
	fmt.Fprintf(out, "[+] Generating reference documentation into '%s'...\n", docsDir)
	res, err := reflect.GenerateReferenceDocs(docsDir, newRootCmd(), buildMCPServer(mcpserver.ProfileFull))
	if err != nil {
		return fmt.Errorf("generate reference docs: %w", err)
	}
	fmt.Fprintf(out, "[✓] Reference documentation generated successfully (%d files: %d CLI, %d MCP)\n",
		res.TotalWritten, len(res.CLIFilesWritten), len(res.MCPFilesWritten))
	return nil
}

var sharingTiers = []docusaurus.SharingTier{
	docusaurus.SharingTierPublic, docusaurus.SharingTierInternal, docusaurus.SharingTierFederated, docusaurus.SharingTierPrivate,
}

func runCodexSite(ctx context.Context, svc docusaurus.Service, o codexGenerateOptions, out io.Writer) error {
	if !slices.Contains(sharingTiers, docusaurus.SharingTier(o.Tier)) {
		return usageErrorf("invalid --tier %q: use public, internal, federated or private", o.Tier)
	}
	cfg, err := docusaurus.NewSiteConfigBuilder().
		WithTitle(o.Title).
		WithPort(o.Port).
		WithSharingTier(docusaurus.SharingTier(o.Tier)).
		Build()
	if err != nil {
		return usageErrorf("site configuration: %v", err)
	}
	fmt.Fprintf(out, "[+] Synthesizing Docusaurus documentation from '%s' into '%s'...\n", o.Docs, o.Out)
	result, err := svc.Generate(ctx, o.Docs, o.Out, cfg)
	if err != nil {
		return fmt.Errorf("site generation: %w", err)
	}
	fmt.Fprintf(out, "[✓] Docusaurus documentation and Project Sharing Hub generated successfully in %dms!\n", result.DurationMs)
	fmt.Fprintf(out, "    - Target Directory: %s\n", result.SiteDirectory)
	fmt.Fprintf(out, "    - Total Documents:  %d\n", result.TotalDocs)
	fmt.Fprintf(out, "    - Categories:       %d\n", result.CategoriesCount)
	fmt.Fprintf(out, "    - Sharing Manifest: %s\n", result.ManifestPath)
	fmt.Fprintf(out, "    - Portable Bundle:  %s\n", result.BundlePath)
	if result.Audit != nil {
		fmt.Fprintf(out, "    - Compliance Score: %.1f%%\n", result.Audit.ComplianceScore*100)
	}
	return nil
}

func newCodexAuditCmd() *cobra.Command {
	var strict, reference bool
	var root, docsDir string
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Audit the reference docs against the code (--strict fails on drift), or the site's coverage",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if strict || reference {
				return runCodexReferenceAudit(docsDir, strict, cmd.OutOrStdout())
			}
			return runCodexSiteAudit(cmd.Context(), docusaurus.NewService(), root, docsDir, cmd.OutOrStdout())
		},
	}
	f := cmd.Flags()
	f.BoolVar(&strict, "strict", false, "Audit the reference docs and exit 1 when any page is missing or stale")
	f.BoolVar(&reference, "reference", false, "Audit the reference docs without failing")
	f.StringVar(&root, "root", ".", "Project root for the site audit")
	f.StringVar(&docsDir, "docs", "docs", "Documentation directory")
	return cmd
}

func runCodexReferenceAudit(docsDir string, strict bool, out io.Writer) error {
	rep, err := reflect.AuditReferenceDocs(docsDir, newRootCmd(), buildMCPServer(mcpserver.ProfileFull))
	if err != nil {
		return fmt.Errorf("reference audit: %w", err)
	}
	if rep.Passed {
		fmt.Fprintln(out, "[✓] Reference documentation audit passed:")
		fmt.Fprintf(out, "    - %d total items checked\n", rep.TotalChecked)
		fmt.Fprintln(out, "    - 0 missing, 0 stale")
		return nil
	}
	rule := strings.Repeat("=", 50)
	fmt.Fprintln(out, rule)
	fmt.Fprintln(out, "       REFERENCE DOCUMENTATION AUDIT FAILED       ")
	fmt.Fprintln(out, rule)
	for _, item := range rep.Items {
		fmt.Fprintf(out, "  [-] [%s] %s (%s): %s\n", strings.ToUpper(item.Reason), item.Type, item.Name, item.Path)
	}
	fmt.Fprintln(out, rule)
	fmt.Fprintf(out, "Total: %d missing, %d stale\n", rep.MissingCount, rep.StaleCount)
	if strict {
		return fmt.Errorf("reference docs are out of date: run 'dendrite codex generate --reference'")
	}
	return nil
}

func runCodexSiteAudit(ctx context.Context, svc docusaurus.Service, root, docsDir string, out io.Writer) error {
	fmt.Fprintln(out, "[+] Running Docusaurus & Project Sharing Documentation Audit...")
	summary, err := svc.Audit(ctx, root, docsDir)
	if err != nil {
		return fmt.Errorf("site audit: %w", err)
	}
	if globals.JSON {
		return writeJSON(out, summary)
	}
	rule := strings.Repeat("=", 50)
	fmt.Fprintln(out, rule)
	fmt.Fprintln(out, "    DOCUSAURUS DOCUMENTATION AUDIT SCORECARD      ")
	fmt.Fprintln(out, rule)
	fmt.Fprintf(out, "Total Features Discovered:     %d\n", summary.TotalFeatures)
	fmt.Fprintf(out, "Features with Acceptance Spec: %d\n", summary.DocumentedFeatures)
	fmt.Fprintf(out, "Sharing Readiness:             %t\n", summary.SharingPrepared)
	if len(summary.UndocumentedModules) > 0 {
		fmt.Fprintf(out, "Undocumented Modules (%d):     %s\n", len(summary.UndocumentedModules), summary.UndocumentedModules)
	}
	if len(summary.Recommendations) > 0 {
		fmt.Fprintln(out, "\nRecommendations:")
		for _, rec := range summary.Recommendations {
			fmt.Fprintf(out, "  * %s\n", rec)
		}
	}
	fmt.Fprintln(out, rule)
	return nil
}

func newCodexReflectCmd() *cobra.Command {
	var refDir, srcDir, targetDir string
	cmd := &cobra.Command{
		Use:   "reflect",
		Short: "Mirror a reference docs taxonomy onto another source tree",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "[+] Reflecting documentation taxonomy from '%s' to '%s' (source: '%s')...\n", refDir, targetDir, srcDir)
			result, err := reflect.ReflectTaxonomy(refDir, srcDir, targetDir)
			if err != nil {
				return fmt.Errorf("reflection: %w", err)
			}
			fmt.Fprintf(out, "[✓] Reflected %d files in %dms (Sidebar Synced: %v)\n", len(result.FilesWritten), result.DurationMs, result.SidebarSynced)
			for _, f := range result.FilesWritten {
				fmt.Fprintf(out, "    - %s\n", f)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&refDir, "reference", "docs", "Reference docs directory")
	cmd.Flags().StringVar(&srcDir, "source", ".", "Source tree to document")
	cmd.Flags().StringVar(&targetDir, "target", "reflected_docs", "Output directory")
	return cmd
}

func addSiteDirFlag(cmd *cobra.Command, p *string) {
	cmd.Flags().StringVar(p, "site", defaultSiteDir, "Docusaurus site directory")
}

// newCodexSiteCmd builds `codex site`: scaffold, export, preview and build the Docusaurus site.
func newCodexSiteCmd() *cobra.Command {
	cmd := newGroupCmd("site", "Scaffold, share, preview and build the Docusaurus site", "", "docusaurus")

	var initDir string
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold a blank Docusaurus project",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "[+] Initializing blank Docusaurus site layout in '%s'...\n", initDir)
			if err := docusaurus.NewService().Init(cmd.Context(), initDir); err != nil {
				return fmt.Errorf("init: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "[✓] Docusaurus skeleton initialized.")
			return nil
		},
	}
	initCmd.Flags().StringVar(&initDir, "out", defaultSiteDir, "Directory to create")

	var shareSite, bundle string
	share := &cobra.Command{
		Use:   "share",
		Short: "Export a portable documentation bundle with a SHA-256 manifest",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "[+] Creating standalone documentation sharing bundle for '%s'...\n", shareSite)
			manifest, err := docusaurus.NewService().ExportBundle(cmd.Context(), shareSite, bundle)
			if err != nil {
				return fmt.Errorf("export sharing bundle: %w", err)
			}
			if globals.JSON {
				return writeJSON(out, manifest)
			}
			fmt.Fprintf(out, "[✓] Successfully exported documentation sharing bundle for '%s' (v%s)!\n", manifest.ProjectName, manifest.Version)
			fmt.Fprintf(out, "    - Documents:  %d\n", manifest.TotalDocuments)
			fmt.Fprintf(out, "    - Categories: %d\n", len(manifest.Categories))
			fmt.Fprintf(out, "    - Commit:     %s (%s)\n", manifest.CommitSHA, manifest.Branch)
			return nil
		},
	}
	addSiteDirFlag(share, &shareSite)
	share.Flags().StringVar(&bundle, "out", "", "Bundle path (default inside the site)")

	var serveSite string
	var port int
	serve := &cobra.Command{
		Use:   "serve",
		Short: "Serve the site preview and sharing portal in the foreground (Ctrl+C stops it)",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return runCodexServe(ctx, docusaurus.NewService(), serveSite, port, cmd.OutOrStdout())
		},
	}
	addSiteDirFlag(serve, &serveSite)
	serve.Flags().IntVar(&port, "port", 3000, "Listen port")

	var buildSite string
	build := &cobra.Command{
		Use:   "build",
		Short: "Run the production npm build of the site",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "[+] Building production Docusaurus static assets in '%s'...\n", buildSite)
			if err := docusaurus.NewService().Build(cmd.Context(), buildSite); err != nil {
				return fmt.Errorf("build: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "[✓] Production build completed.")
			return nil
		},
	}
	addSiteDirFlag(build, &buildSite)

	cmd.AddCommand(initCmd, share, serve, build)
	return cmd
}

func runCodexServe(ctx context.Context, svc docusaurus.Service, site string, port int, out io.Writer) error {
	fmt.Fprintf(out, "[+] Launching Docusaurus documentation and sharing preview server on :%d...\n", port)
	inst, err := svc.Serve(ctx, site, port)
	if err != nil {
		return fmt.Errorf("start preview server: %w", err)
	}
	defer func() { _ = inst.Shutdown(context.Background()) }()
	fmt.Fprintf(out, "[✓] Serving site at http://localhost:%d (Press Ctrl+C to terminate)\n", port)
	fmt.Fprintf(out, "    - Health:   http://localhost:%d/api/v1/health\n", port)
	fmt.Fprintf(out, "    - Manifest: http://localhost:%d/api/v1/sharing-manifest\n", port)
	<-ctx.Done()
	fmt.Fprintln(out, "\n[+] Shutting down preview server...")
	return nil
}

// newCodexPlaneCmd builds `codex plane`: sync the docs directory with Plane pages.
func newCodexPlaneCmd() *cobra.Command {
	cmd := newGroupCmd("plane", "Sync the docs directory with Plane pages", "")
	var dir string
	cmd.PersistentFlags().StringVarP(&dir, "dir", "d", "docs", "Documentation directory")
	for _, d := range []struct{ use, short string }{
		{"push", "Publish local docs as Plane pages"},
		{"pull", "Write Plane pages into the docs directory"},
		{"sync", "Push and pull, newest wins"},
	} {
		direction := d.use
		cmd.AddCommand(&cobra.Command{
			Use:   direction,
			Short: d.short,
			Args:  noArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return runPlaneDocSync(cmd.Context(), direction, dir, cmd.OutOrStdout())
			},
		})
	}
	return cmd
}
