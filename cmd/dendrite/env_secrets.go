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
	var generate, rotate bool
	cmd := &cobra.Command{
		Use:   "ensure",
		Short: "Create .env from stacks/.env.example and optionally change credentials",
		Long: "Creates .env (owner-only, never committed) from the committed defaults in stacks/.env.example and adds any key\n" +
			"a newer template introduced. Existing values are never overwritten, so re-running is safe.\n\n" +
			"The defaults are local-development values, published on purpose. Change them with --set KEY=VALUE, answer the\n" +
			"prompts (in a terminal), or use --generate for random values on every key still at its default. --rotate also\n" +
			"replaces custom values. Docker publishes Postgres, Redis, LiteLLM and Plane on all interfaces, so change the\n" +
			"defaults on any machine other people can reach.",
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
			opts := env.SecretsOptions{Dir: dotenv.ProjectRoot("."), Set: set, Generate: generate, Rotate: rotate}
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
		fmt.Fprintf(out, "[✓] Created %s from %s\n", res.Path, env.TemplatePath)
	case len(res.Added)+len(res.Changed) == 0:
		fmt.Fprintf(out, "[✓] %s is up to date\n", res.Path)
	default:
		fmt.Fprintf(out, "[✓] Updated %s\n", res.Path)
	}
	if len(res.Added) > 0 {
		fmt.Fprintf(out, "    added:   %s\n", strings.Join(res.Added, ", "))
	}
	if len(res.Changed) > 0 {
		fmt.Fprintf(out, "    changed: %s\n", strings.Join(res.Changed, ", "))
	}
	if len(res.StillDefault) > 0 {
		fmt.Fprintf(out, "[!] %d credential(s) still use the published defaults (see %s): %s\n",
			len(res.StillDefault), env.TemplatePath, strings.Join(res.StillDefault, ", "))
		fmt.Fprintln(out, "    Fine while only you can reach this machine; otherwise run `dendrite soma secrets ensure --generate`.")
	}
	return nil
}

func newSomaSecretsStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show which credentials are the published default, custom or missing (never prints values)",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := env.SecretState(dotenv.ProjectRoot("."))
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
