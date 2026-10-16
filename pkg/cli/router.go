package cli

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
)

// CommandHandler represents the execution logic of a CLI command.
type CommandHandler func(ctx context.Context, args []string) error

// Command represents a modular CLI command or sub-command.
type Command struct {
	Name        string
	Aliases     []string
	Category    string
	Description string
	Usage       string
	Run         CommandHandler
	Subcommands map[string]*Command
}

// Registry manages the set of registered CLI commands and aliases.
type Registry struct {
	commands        map[string]*Command // maps primary names and aliases to *Command
	primaryCommands []*Command          // ordered list of registered primary commands
	categories      []string            // unique categories in order of appearance
}

// NewRegistry initializes an empty Command Registry.
func NewRegistry() *Registry {
	return &Registry{
		commands:        make(map[string]*Command),
		primaryCommands: make([]*Command, 0),
		categories:      make([]string, 0),
	}
}

// Register adds a command to the registry along with all its aliases.
func (r *Registry) Register(cmd *Command) {
	if cmd == nil || cmd.Name == "" {
		return
	}

	// Register primary command name
	r.commands[strings.ToLower(cmd.Name)] = cmd
	r.primaryCommands = append(r.primaryCommands, cmd)

	// Register all aliases
	for _, alias := range cmd.Aliases {
		if alias != "" {
			r.commands[strings.ToLower(alias)] = cmd
		}
	}

	// Track category order
	if cmd.Category != "" {
		found := false
		for _, cat := range r.categories {
			if cat == cmd.Category {
				found = true
				break
			}
		}
		if !found {
			r.categories = append(r.categories, cmd.Category)
		}
	}
}

// Lookup finds a command by name or alias.
func (r *Registry) Lookup(name string) (*Command, bool) {
	cmd, ok := r.commands[strings.ToLower(name)]
	return cmd, ok
}

// Execute resolves and executes a command matching the provided arguments.
func (r *Registry) Execute(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no command specified")
	}

	name := args[0]
	cmd, ok := r.Lookup(name)
	if !ok {
		suggestions := r.Suggest(name)
		if len(suggestions) > 0 {
			return fmt.Errorf("unknown command '%s'. Did you mean: %s?", name, strings.Join(suggestions, ", "))
		}
		return fmt.Errorf("unknown command '%s'", name)
	}

	if cmd.Run == nil {
		return fmt.Errorf("command '%s' has no executable runner", name)
	}

	return cmd.Run(ctx, args[1:])
}

// ListPrimary returns all registered primary commands.
func (r *Registry) ListPrimary() []*Command {
	return append([]*Command(nil), r.primaryCommands...)
}

// Categories returns the distinct registered command categories.
func (r *Registry) Categories() []string {
	return append([]string(nil), r.categories...)
}

// Suggest finds close matches for a misspelled command using prefix and edit distance.
func (r *Registry) Suggest(input string) []string {
	input = strings.ToLower(input)

	type match struct {
		name string
		dist int
	}
	var matches []match

	for name := range r.commands {
		// Exact prefix match
		if strings.HasPrefix(name, input) || strings.HasPrefix(input, name) {
			matches = append(matches, match{name: name, dist: 0})
			continue
		}

		dist := levenshteinDistance(input, name)
		if dist <= 2 {
			matches = append(matches, match{name: name, dist: dist})
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].dist != matches[j].dist {
			return matches[i].dist < matches[j].dist
		}
		return matches[i].name < matches[j].name
	})

	var result []string
	for _, m := range matches {
		if !contains(result, m.name) {
			result = append(result, m.name)
		}
		if len(result) >= 3 {
			break
		}
	}
	return result
}

// PrintUsage writes a formatted, categorized help overview.
func (r *Registry) PrintUsage(w io.Writer, banner string) {
	if banner != "" {
		fmt.Fprintln(w, banner)
	}
	fmt.Fprintln(w, "\nUsage:")
	fmt.Fprintln(w, "  dendrite [command] [args...]")
	fmt.Fprintln(w, "\nAvailable Commands (Neuro-Lore & short aliases supported):")

	if len(r.categories) > 0 {
		for _, cat := range r.categories {
			fmt.Fprintf(w, "\n  [%s]\n", cat)
			for _, cmd := range r.primaryCommands {
				if cmd.Category == cat {
					aliases := ""
					if len(cmd.Aliases) > 0 {
						aliases = fmt.Sprintf(" (aliases: %s)", strings.Join(cmd.Aliases, ", "))
					}
					fmt.Fprintf(w, "    %-26s %s%s\n", cmd.Name, cmd.Description, aliases)
				}
			}
		}
	} else {
		for _, cmd := range r.primaryCommands {
			aliases := ""
			if len(cmd.Aliases) > 0 {
				aliases = fmt.Sprintf(" (aliases: %s)", strings.Join(cmd.Aliases, ", "))
			}
			fmt.Fprintf(w, "  %-26s %s%s\n", cmd.Name, cmd.Description, aliases)
		}
	}
}

func contains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

// levenshteinDistance computes edit distance between two strings.
func levenshteinDistance(s1, s2 string) int {
	r1, r2 := []rune(s1), []rune(s2)
	n, m := len(r1), len(r2)
	if n == 0 {
		return m
	}
	if m == 0 {
		return n
	}

	matrix := make([][]int, n+1)
	for i := range matrix {
		matrix[i] = make([]int, m+1)
		matrix[i][0] = i
	}
	for j := 0; j <= m; j++ {
		matrix[0][j] = j
	}

	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			cost := 0
			if r1[i-1] != r2[j-1] {
				cost = 1
			}
			del := matrix[i-1][j] + 1
			ins := matrix[i][j-1] + 1
			sub := matrix[i-1][j-1] + cost

			min := del
			if ins < min {
				min = ins
			}
			if sub < min {
				min = sub
			}
			matrix[i][j] = min
		}
	}

	return matrix[n][m]
}
