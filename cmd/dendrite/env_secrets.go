package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"Dendrite/pkg/dotenv"
	"Dendrite/pkg/env"

	"github.com/spf13/cobra"
)

func newSomaSecretsCmd() *cobra.Command {
	cmd := newGroupCmd("secrets", "Create .env from the published defaults and change them at install", "")
	cmd.Example = "dendrite soma secrets ensure\n" +
		"dendrite soma secrets ensure --set POSTGRES_PASSWORD=mine --set LITELLM_MASTER_KEY=sk-mine\n" +
		"dendrite soma secrets ensure --generate\n" +
		"dendrite soma secrets status"
	cmd.AddCommand(newSomaSecretsEnsureCmd(), newSomaSecretsStatusCmd())
	return cmd
}

func newSomaSecretsEnsureCmd() *cobra.Command {
	var sets []string
	var generate, rotate, local bool
	cmd := &cobra.Command{
		Use:   "ensure",
		Short: "Set credentials once in ~/.dendrite/.env; the committed .env keeps the published defaults",
		Long: "The committed .env holds the published defaults (local-development values, public on purpose; stacks/.env.example\n" +
			"is the template). This command never edits it. Your own values go to $DENDRITE_HOME/.env, configured once and used\n" +
			"by every project (it outranks the committed .env); --local writes this checkout's untracked .env.local instead.\n" +
			"Only the keys being set are touched, and existing values are never overwritten, so re-running is safe.\n\n" +
			"Change a value with --set KEY=VALUE, answer the prompts (in a terminal), or use --generate for random values on\n" +
			"every key still at its default. --rotate also replaces custom values. Docker publishes Postgres, Redis, LiteLLM\n" +
			"and Plane on all interfaces, so change the defaults on any machine other people can reach.",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			set := map[string]string{}
			for _, kv := range sets {
				k, v, ok := strings.Cut(kv, "=")
				if !ok || strings.TrimSpace(k) == "" {
					return usageErrorf("--set wants KEY=VALUE, got %q", kv)
				}
				set[strings.TrimSpace(k)] = v
			}
			opts := env.SecretsOptions{Dir: dotenv.ProjectRoot("."), GlobalPath: globalEnvPath(), Local: local, Set: set, Generate: generate, Rotate: rotate}
			if len(set) == 0 && !generate && !globals.Yes && stdinIsTerminal() {
				opts.Prompt = promptSecret(os.Stdin, cmd.OutOrStdout())
			}
			res, err := env.EnsureSecrets(opts)
			if err != nil {
				return err
			}
			return reportSecrets(cmd.OutOrStdout(), res)
		},
	}
	cmd.Flags().StringArrayVar(&sets, "set", nil, "Set KEY=VALUE (repeatable)")
	cmd.Flags().BoolVar(&generate, "generate", false, "Random values for every managed key still at its published default")
	cmd.Flags().BoolVar(&rotate, "rotate", false, "With --generate: replace custom values too")
	cmd.Flags().BoolVar(&local, "local", false, "Write to this checkout's .env.local instead of the global file")
	return cmd
}

// promptSecret asks about each managed key; Enter keeps the current value. Values typed here are
// echoed, as they are for any setting; use --set from a shell with history off for secrets you
// want to keep out of the terminal.
func promptSecret(in io.Reader, out io.Writer) func(string, bool) (string, error) {
	r := bufio.NewReader(in)
	return func(key string, isDefault bool) (string, error) {
		state := "custom"
		if isDefault {
			state = "published default"
		}
		fmt.Fprintf(out, "%s (%s) - new value, or Enter to keep: ", key, state)
		line, err := r.ReadString('\n')
		if err != nil && err != io.EOF {
			return "", err
		}
		return strings.TrimSpace(line), nil
	}
}

func reportSecrets(out io.Writer, res *env.SecretsResult) error {
	if globals.JSON {
		return writeJSON(out, res)
	}
	switch {
	case res.Created:
		fmt.Fprintf(out, "[✓] Wrote %s (owner-only)\n", res.Path)
	case len(res.Added)+len(res.Changed) > 0:
		fmt.Fprintf(out, "[✓] Updated %s\n", res.Path)
	default:
		fmt.Fprintln(out, "[✓] Nothing to change: your global file, .env.local and the committed defaults already cover every credential")
	}
	if len(res.Added) > 0 {
		fmt.Fprintf(out, "    added:   %s\n", strings.Join(res.Added, ", "))
	}
	if len(res.Changed) > 0 {
		fmt.Fprintf(out, "    changed: %s\n", strings.Join(res.Changed, ", "))
	}
	if len(res.ShadowedByLocal) > 0 {
		fmt.Fprintf(out, "[!] This checkout's .env.local overrides %s, so the new global value does not apply here.\n", strings.Join(res.ShadowedByLocal, ", "))
	}
	if len(res.StillDefault) > 0 {
		fmt.Fprintf(out, "[!] %d credential(s) still use the published defaults (see %s): %s\n",
			len(res.StillDefault), env.TemplatePath, strings.Join(res.StillDefault, ", "))
		fmt.Fprintln(out, "    Fine while only you can reach this machine; otherwise run `dendrite soma secrets ensure --generate`.")
	}
	return nil
}

// globalEnvPath is $DENDRITE_HOME/.env, or "" when the home cannot be resolved.
func globalEnvPath() string {
	p, err := dotenv.GlobalPath()
	if err != nil {
		return ""
	}
	return p
}

func newSomaSecretsStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show which credentials are the published default, custom or missing (never prints values)",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := env.SecretState(dotenv.ProjectRoot("."), globalEnvPath())
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if globals.JSON {
				return writeJSON(out, st)
			}
			for _, k := range env.ManagedSecrets {
				fmt.Fprintf(out, "    %-24s %s\n", k, st[k])
			}
			return nil
		},
	}
}
