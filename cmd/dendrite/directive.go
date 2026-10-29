package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"Dendrite/pkg/directive"
	"Dendrite/pkg/llmchat"
)

// directiveChat is the model that reads rules the parser does not know; a seam for tests.
var directiveChat = func(dir string) llmchat.Completer { return llmchat.Gateway(dir) }

func newDirectiveCmd() *cobra.Command {
	var path string
	cmd := &cobra.Command{
		Use:     "directive [rule]",
		Aliases: []string{"directives"},
		GroupID: "glia",
		Short:   "Change what CI, the quality gate and the agents enforce: lines per file, coverage, duplication, who sets Done",
		Long: "Reads a rule, shows what it changes in " + directive.Path + ", asks to confirm, saves it and regenerates the files\n" +
			"derived from it (codecov.yml, .jscpd.json, and the CI and lint files as they join). Rules it knows are read\n" +
			"directly (\"max 500 lines of code\", \"95% code coverage\", \"zero duplicated code\", \"allow agents to set done\",\n" +
			"\"require human review\"); any other is read by the local model, and anything it proposes is checked against the\n" +
			"settings and their limits before it is written. `directive set key=value` skips the reading.",
		Example: "dendrite directive \"enforce max 500 lines of code ignoring tests\"\n" +
			"dendrite directive \"enforce 95% code coverage\" -y\n" +
			"dendrite directive set coverage.min_percent=95 loc.warn_at=400\n" +
			"dendrite directive list",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			return runDirectiveRule(cmd.Context(), path, strings.Join(args, " "), cmd.InOrStdin(), cmd.OutOrStdout())
		},
	}
	cmd.PersistentFlags().StringVar(&path, "path", ".", "Project directory")
	cmd.AddCommand(
		&cobra.Command{
			Use: "list", Short: "Show the directives in force", Args: noArgs,
			RunE: func(cmd *cobra.Command, args []string) error { return runDirectiveList(path, cmd.OutOrStdout()) },
		},
		&cobra.Command{
			Use: "set <key=value>...", Short: "Set directives directly, without reading a rule", Args: cobra.MinimumNArgs(1),
			Example: "dendrite directive set coverage.min_percent=95 loc.warn_at=400",
			RunE: func(cmd *cobra.Command, args []string) error {
				return runDirectiveSet(path, args, cmd.InOrStdin(), cmd.OutOrStdout())
			},
		},
		&cobra.Command{
			Use: "apply", Short: "Regenerate the files derived from the directives", Args: noArgs,
			RunE: func(cmd *cobra.Command, args []string) error { return runDirectiveApply(path, cmd.OutOrStdout()) },
		},
	)
	return cmd
}

func runDirectiveRule(ctx context.Context, dir, rule string, in io.Reader, out io.Writer) error {
	changes := directive.Parse(rule)
	if len(changes) == 0 {
		fmt.Fprintln(out, "[i] Not a rule I know by heart; asking the local model...")
		var err error
		if changes, err = directive.Interpret(ctx, directiveChat(dir), rule); err != nil {
			return err
		}
	}
	return changeDirectives(dir, changes, in, out)
}

func runDirectiveSet(dir string, pairs []string, in io.Reader, out io.Writer) error {
	var changes []directive.Change
	for _, pair := range pairs {
		key, text, ok := strings.Cut(pair, "=")
		if !ok {
			return usageErrorf("%q is not key=value", pair)
		}
		value, err := directive.ParseValue(strings.TrimSpace(key), text)
		if err != nil {
			return usageErrorf("%v", err)
		}
		changes = append(changes, directive.Change{Key: strings.TrimSpace(key), Value: value})
	}
	return changeDirectives(dir, changes, in, out)
}

// changeDirectives shows what the changes do to the directives, asks to confirm, then saves and regenerates.
func changeDirectives(dir string, changes []directive.Change, in io.Reader, out io.Writer) error {
	before, err := directive.Load(dir)
	if err != nil {
		return err
	}
	after := before
	if err := after.ApplyAll(changes); err != nil {
		return err
	}
	diff := directive.Diff(string(directive.Encode(before)), string(directive.Encode(after)))
	if diff == "" && directive.Exists(dir) {
		fmt.Fprintln(out, "[✓] Already so; nothing to change.")
		return nil
	}
	for _, c := range changes {
		fmt.Fprintf(out, "    - %s\n", c)
	}
	if !directive.Exists(dir) {
		fmt.Fprintf(out, "[i] %s does not exist yet; it will be created.\n", directive.Path)
	}
	fmt.Fprint(out, indentText(diff, "    "))
	if !globals.Yes {
		if !isTerminal(in) {
			return usageErrorf("this changes %s: run it in a terminal to confirm, or pass -y", directive.Path)
		}
		fmt.Fprint(out, "[?] Apply? [y/N] ")
		answer, _ := bufio.NewReader(in).ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(answer)); a != "y" && a != "yes" {
			fmt.Fprintln(out, "[i] Left as it was.")
			return nil
		}
	}
	if err := directive.Save(dir, after); err != nil {
		return err
	}
	fmt.Fprintf(out, "[✓] Saved %s\n", directive.Path)
	return renderDirectives(dir, after, out)
}

func renderDirectives(dir string, set directive.Set, out io.Writer) error {
	res, err := directive.Render(dir, set)
	for _, f := range res.Written {
		fmt.Fprintf(out, "[✓] Regenerated %s\n", f)
	}
	for _, f := range res.Kept {
		fmt.Fprintf(out, "[i] Kept the hand-written %s\n", f)
	}
	return err
}

func runDirectiveList(dir string, out io.Writer) error {
	set, err := directive.Load(dir)
	if err != nil {
		return err
	}
	if !directive.Exists(dir) {
		fmt.Fprintf(out, "# no %s yet: these are the defaults\n", directive.Path)
	}
	fmt.Fprint(out, string(directive.Encode(set)))
	return nil
}

func runDirectiveApply(dir string, out io.Writer) error {
	set, err := directive.Load(dir)
	if err != nil {
		return err
	}
	if !directive.Exists(dir) {
		if err := directive.Save(dir, set); err != nil {
			return err
		}
		fmt.Fprintf(out, "[✓] Created %s with the defaults\n", filepath.ToSlash(directive.Path))
	}
	return renderDirectives(dir, set, out)
}
