package reflect

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// SubcommandDoc describes a (possibly nested) subcommand of a CLI command.
type SubcommandDoc struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Usage       string    `json:"usage,omitempty"`
	Aliases     []string  `json:"aliases,omitempty"`
	Flags       []FlagDoc `json:"flags,omitempty"`
	Examples    []string  `json:"examples,omitempty"`
}

// FlagDoc describes a CLI command line flag.
type FlagDoc struct {
	Name        string `json:"name"`
	Type        string `json:"type,omitempty"`
	Description string `json:"description"`
	Default     string `json:"default,omitempty"`
}

// CLICommandDoc holds structured documentation data for a primary CLI command.
type CLICommandDoc struct {
	Name        string          `json:"name"`
	Category    string          `json:"category"`
	Description string          `json:"description"`
	Aliases     []string        `json:"aliases"`
	Subcommands []SubcommandDoc `json:"subcommands,omitempty"`
	Flags       []FlagDoc       `json:"flags,omitempty"`
	Examples    []string        `json:"examples,omitempty"`
}

// CLIRootDoc documents the root command: its global flags and the command groups.
type CLIRootDoc struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	GlobalFlags []FlagDoc         `json:"global_flags,omitempty"`
	Groups      []CLIRootGroupDoc `json:"groups,omitempty"`
}

// CLIRootGroupDoc lists the commands of one help group.
type CLIRootGroupDoc struct {
	Title    string          `json:"title"`
	Commands []SubcommandDoc `json:"commands"`
}

// documentedCommands returns the user-facing children of cmd (no help, completion or hidden commands).
func documentedCommands(cmd *cobra.Command) []*cobra.Command {
	var out []*cobra.Command
	for _, c := range cmd.Commands() {
		if c.Hidden || c.Name() == "help" || c.Name() == "completion" {
			continue
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

// IntrospectCLITree introspects a live cobra command tree and extracts documentation models for all primary commands.
func IntrospectCLITree(root *cobra.Command) []CLICommandDoc {
	if root == nil {
		return nil
	}
	cmds := documentedCommands(root)
	docs := make([]CLICommandDoc, 0, len(cmds))
	for _, cmd := range cmds {
		docs = append(docs, IntrospectCLICommand(cmd))
	}
	return docs
}

// IntrospectCLICommand extracts documentation metadata from a *cobra.Command and its nested subcommands.
func IntrospectCLICommand(cmd *cobra.Command) CLICommandDoc {
	if cmd == nil {
		return CLICommandDoc{}
	}

	doc := CLICommandDoc{
		Name:        cmd.Name(),
		Category:    categoryOf(cmd),
		Description: cmd.Short,
		Aliases:     append([]string(nil), cmd.Aliases...),
		Flags:       flagDocs(cmd.LocalNonPersistentFlags()),
		Examples:    exampleLines(cmd.Example),
	}

	var walk func(c *cobra.Command, prefix string)
	walk = func(c *cobra.Command, prefix string) {
		for _, sub := range documentedCommands(c) {
			name := strings.TrimSpace(prefix + " " + sub.Name())
			doc.Subcommands = append(doc.Subcommands, SubcommandDoc{
				Name:        name,
				Description: sub.Short,
				Usage:       sub.UseLine(),
				Aliases:     append([]string(nil), sub.Aliases...),
				Flags:       flagDocs(sub.LocalNonPersistentFlags()),
				Examples:    exampleLines(sub.Example),
			})
			walk(sub, name)
		}
	}
	walk(cmd, "")

	enrichCommandDoc(&doc)
	return doc
}

// IntrospectCLIRoot documents the root command's global flags and its command groups.
func IntrospectCLIRoot(root *cobra.Command) CLIRootDoc {
	if root == nil {
		return CLIRootDoc{}
	}
	doc := CLIRootDoc{
		Name:        root.Name(),
		Description: root.Short,
		GlobalFlags: flagDocs(root.PersistentFlags()),
	}
	byGroup := map[string][]SubcommandDoc{}
	for _, c := range documentedCommands(root) {
		byGroup[c.GroupID] = append(byGroup[c.GroupID], SubcommandDoc{Name: c.Name(), Description: c.Short, Aliases: c.Aliases})
	}
	for _, g := range root.Groups() {
		if cmds := byGroup[g.ID]; len(cmds) > 0 {
			doc.Groups = append(doc.Groups, CLIRootGroupDoc{Title: strings.TrimSuffix(g.Title, ":"), Commands: cmds})
		}
	}
	if cmds := byGroup[""]; len(cmds) > 0 {
		doc.Groups = append(doc.Groups, CLIRootGroupDoc{Title: "Other", Commands: cmds})
	}
	return doc
}

// categoryOf returns the help-group title of a top-level command.
func categoryOf(cmd *cobra.Command) string {
	if cmd.GroupID == "" || !cmd.HasParent() {
		return ""
	}
	for _, g := range cmd.Parent().Groups() {
		if g.ID == cmd.GroupID {
			return strings.TrimSuffix(g.Title, ":")
		}
	}
	return ""
}

// flagDocs converts a pflag set into sorted FlagDocs, skipping the built-in help flag.
func flagDocs(fs *pflag.FlagSet) []FlagDoc {
	var out []FlagDoc
	fs.VisitAll(func(f *pflag.Flag) {
		if f.Name == "help" || f.Hidden {
			return
		}
		name := "--" + f.Name
		if f.Shorthand != "" {
			name += ", -" + f.Shorthand
		}
		def := f.DefValue
		switch def {
		case "false", "[]", "0", "0s":
			def = ""
		}
		out = append(out, FlagDoc{Name: name, Type: f.Value.Type(), Description: f.Usage, Default: def})
	})
	return out
}

// exampleLines splits a cobra Example block into trimmed, non-empty lines.
func exampleLines(example string) []string {
	var out []string
	for _, l := range strings.Split(example, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// RenderCLIRootMarkdown renders docs/cli/dendrite.md: global flags and the grouped command index.
func RenderCLIRootMarkdown(doc CLIRootDoc) string {
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("id: %s\n", doc.Name))
	sb.WriteString(fmt.Sprintf("title: \"%s\"\n", doc.Name))
	sb.WriteString(fmt.Sprintf("sidebar_label: \"%s\"\n", doc.Name))
	sb.WriteString("sidebar_position: 1\n")
	sb.WriteString("---\n\n")
	sb.WriteString(fmt.Sprintf("# %s\n\n", doc.Name))
	if doc.Description != "" {
		sb.WriteString(doc.Description + "\n\n")
	}
	sb.WriteString("Commands use Neuro-Lore names; the plain names listed as aliases work too.\n\n")

	sb.WriteString("## Global Flags\n\n")
	writeFlagTable(&sb, doc.GlobalFlags)

	sb.WriteString("## Commands\n\n")
	for _, g := range doc.Groups {
		sb.WriteString(fmt.Sprintf("### %s\n\n", g.Title))
		sb.WriteString("| Command | Aliases | Description |\n")
		sb.WriteString("| :--- | :--- | :--- |\n")
		for _, c := range g.Commands {
			sb.WriteString(fmt.Sprintf("| [`%s`](%s.md) | %s | %s |\n", c.Name, c.Name, formatAliases(c.Aliases), c.Description))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func formatAliases(aliases []string) string {
	if len(aliases) == 0 {
		return ""
	}
	return "`" + strings.Join(aliases, "`, `") + "`"
}

// writeFlagTable writes a flag table, or "None" when there are no flags.
func writeFlagTable(sb *strings.Builder, flags []FlagDoc) {
	if len(flags) == 0 {
		sb.WriteString("None\n\n")
		return
	}
	sb.WriteString("| Flag | Description |\n")
	sb.WriteString("| :--- | :--- |\n")
	for _, flag := range flags {
		desc := flag.Description
		if flag.Default != "" {
			desc = fmt.Sprintf("%s (default: %s)", desc, flag.Default)
		}
		sb.WriteString(fmt.Sprintf("| `%s` | %s |\n", flag.Name, desc))
	}
	sb.WriteString("\n")
}

// RenderCLICommandMarkdown generates the markdown documentation string for a CLI command.
func RenderCLICommandMarkdown(doc CLICommandDoc) string {
	var sb strings.Builder

	// Frontmatter
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("id: %s\n", doc.Name))
	sb.WriteString(fmt.Sprintf("title: \"dendrite %s\"\n", doc.Name))
	sb.WriteString(fmt.Sprintf("sidebar_label: \"%s\"\n", doc.Name))
	sb.WriteString("---\n\n")

	// Heading & Description
	sb.WriteString(fmt.Sprintf("# dendrite %s\n\n", doc.Name))
	if doc.Description != "" {
		sb.WriteString(doc.Description + "\n\n")
	} else {
		sb.WriteString("No description available.\n\n")
	}

	// Category
	sb.WriteString("## Category\n\n")
	if doc.Category != "" {
		sb.WriteString(doc.Category + "\n\n")
	} else {
		sb.WriteString("General\n\n")
	}

	// Aliases
	sb.WriteString("## Aliases\n\n")
	if len(doc.Aliases) > 0 {
		for _, alias := range doc.Aliases {
			sb.WriteString(fmt.Sprintf("- `%s`\n", alias))
		}
		sb.WriteString("\n")
	} else {
		sb.WriteString("None\n\n")
	}

	// Subcommands
	sb.WriteString("## Subcommands\n\n")
	if len(doc.Subcommands) > 0 {
		sb.WriteString("| Subcommand | Description |\n")
		sb.WriteString("| :--- | :--- |\n")
		for _, sub := range doc.Subcommands {
			sb.WriteString(fmt.Sprintf("| `%s` | %s |\n", sub.Name, sub.Description))
		}
		sb.WriteString("\n")
	} else {
		sb.WriteString("None\n\n")
	}

	// Flags
	sb.WriteString("## Flags\n\n")
	writeFlagTable(&sb, doc.Flags)

	// Subcommand reference: usage, flags and examples of every subcommand that declares any
	var detailed []SubcommandDoc
	for _, sub := range doc.Subcommands {
		if len(sub.Flags) > 0 || len(sub.Examples) > 0 || len(sub.Aliases) > 0 {
			detailed = append(detailed, sub)
		}
	}
	if len(detailed) > 0 {
		sb.WriteString("## Subcommand Reference\n\n")
		for _, sub := range detailed {
			sb.WriteString(fmt.Sprintf("### `dendrite %s %s`\n\n", doc.Name, sub.Name))
			if sub.Description != "" {
				sb.WriteString(sub.Description + "\n\n")
			}
			if sub.Usage != "" {
				sb.WriteString(fmt.Sprintf("```bash\n%s\n```\n\n", sub.Usage))
			}
			if len(sub.Aliases) > 0 {
				sb.WriteString("Aliases: " + formatAliases(sub.Aliases) + "\n\n")
			}
			if len(sub.Flags) > 0 {
				writeFlagTable(&sb, sub.Flags)
			}
			if len(sub.Examples) > 0 {
				sb.WriteString("```bash\n" + strings.Join(sub.Examples, "\n") + "\n```\n\n")
			}
		}
	}

	// Examples
	sb.WriteString("## Examples\n\n")
	if len(doc.Examples) > 0 {
		sb.WriteString("```bash\n")
		for _, ex := range doc.Examples {
			sb.WriteString(ex + "\n")
		}
		sb.WriteString("```\n")
	} else {
		sb.WriteString(fmt.Sprintf("```bash\ndendrite %s\n```\n", doc.Name))
	}

	return sb.String()
}

// enrichCommandDoc fills subcommands, flags and examples from a static catalog for commands that are not
// yet native cobra trees (legacy adapters, DEN-950). The catalog is keyed by plain name; any alias matches.
func enrichCommandDoc(doc *CLICommandDoc) {
	catalog := map[string]struct {
		Subcommands []SubcommandDoc
		Flags       []FlagDoc
		Examples    []string
	}{
	}

	entry, found := catalog[doc.Name]
	for _, alias := range doc.Aliases {
		if found {
			break
		}
		entry, found = catalog[alias]
	}
	if found {
		if len(doc.Subcommands) == 0 && len(entry.Subcommands) > 0 {
			doc.Subcommands = append([]SubcommandDoc(nil), entry.Subcommands...)
		}
		if len(doc.Flags) == 0 && len(entry.Flags) > 0 {
			doc.Flags = append([]FlagDoc(nil), entry.Flags...)
		}
		if len(doc.Examples) == 0 && len(entry.Examples) > 0 {
			doc.Examples = append([]string(nil), entry.Examples...)
		}
	}

	if len(doc.Examples) == 0 {
		doc.Examples = []string{fmt.Sprintf("dendrite %s", doc.Name)}
	}
}
