package main

import (
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"os"
)

const version = "0.1.0-alpha"

func main() {
	err := newRootCmd().ExecuteContext(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "[-] %v\n", err)
	}
	os.Exit(exitCodeFor(err))
}

// legacyCommands returns the commands whose handlers still parse their own arguments (DEN-950).
func legacyCommands() []*cobra.Command {
	return []*cobra.Command{
		legacyCmd(legacySpec{
			Name:        "inhibitory-triage",
			Aliases:     []string{"triage", "trg"},
			Category:    "Postsynaptic Effectors & Actuation",
			Description: "Resolve local agent task triage and priority routing",
			Run: func(ctx context.Context, args []string) error {
				handleTriageCommand(ctx, args)
				return nil
			},
		}),
		legacyCmd(legacySpec{
			Name:        "reflex",
			Aliases:     []string{"test", "myotatic-reflex", "tst"},
			Category:    "Postsynaptic Effectors & Actuation",
			Description: "Table-driven test synthesis, Testcontainers & UI tests",
			Run: func(ctx context.Context, args []string) error {
				if len(args) < 1 {
					printTestUsage()
					return nil
				}
				handleTestCommand(ctx, args[0], args[1:])
				return nil
			},
		}),
		legacyCmd(legacySpec{
			Name:        "codex",
			Aliases:     []string{"doc", "docs", "engram-codex"},
			Category:    "Cortex & Sensory Integration",
			Description: "Docs-as-Code reflection, Docusaurus synthesis and portal",
			Run: func(ctx context.Context, args []string) error {
				if len(args) < 1 {
					fmt.Println("Usage: dendrite doc <generate|audit|docusaurus|reflect|plane> [args...]")
					return nil
				}
				handleDocCommand(ctx, args[0], args[1:])
				return nil
			},
		}),
		legacyCmd(legacySpec{
			Name:        "cortex",
			Aliases:     []string{"ui", "cortex-cockpit"},
			Category:    "Cortex & Sensory Integration",
			Description: "Embedded Web Control Cockpit and telemetry server",
			Run: func(ctx context.Context, args []string) error {
				handleUICommand(ctx, args)
				return nil
			},
		}),
		legacyCmd(legacySpec{
			Name:        "sensory",
			Aliases:     []string{"telemetry", "afferent-telemetry", "tel"},
			Category:    "Cortex & Sensory Integration",
			Description: "Hardware thermal, token spend, and Prometheus telemetry",
			Run: func(ctx context.Context, args []string) error {
				handleTelemetryCommand(ctx, args)
				return nil
			},
		}),
		legacyCmd(legacySpec{
			Name:        "dev",
			Category:    "Soma & Axon Hillock",
			Description: "Developer diagnostics, local Ollama probe, and ticket tools",
			Run: func(ctx context.Context, args []string) error {
				if len(args) < 1 {
					fmt.Println("Usage: dendrite dev <ollama|ticket|topology|gateway|audit-prompt> [args...]")
					return nil
				}
				handleDevCommand(ctx, args[0], args[1:])
				return nil
			},
		}),
		legacyCmd(legacySpec{
			Name:        "bridge",
			Aliases:     []string{"jetbrains", "neuromuscular-bridge", "jb"},
			Category:    "Synaptic Cleft & Gating",
			Description: "JetBrains IDE MCP protocol integration and PSI symbol indexer",
			Run: func(ctx context.Context, args []string) error {
				if len(args) < 1 {
					fmt.Println("Usage: dendrite jetbrains <probe|open|problems|symbols|build|run|setup> [args...]")
					return nil
				}
				handleJetBrainsCommand(ctx, args[0], args[1:])
				return nil
			},
		}),
		legacyCmd(legacySpec{
			Name:        "engram",
			Aliases:     []string{"store", "engram-store", "gram"},
			Category:    "Soma & Axon Hillock",
			Description: "Persistent segmented store, memory checkpointer, and chunk query",
			Run: func(ctx context.Context, args []string) error {
				if len(args) < 1 {
					printStoreUsage()
					return nil
				}
				handleStoreCommand(ctx, args[0], args[1:])
				return nil
			},
		}),
		legacyCmd(legacySpec{
			Name:        "cognition",
			Aliases:     []string{"analyze", "cognitive-analysis", "anlz"},
			Category:    "Glial Regulation & Pruning",
			Description: "Cognitive complexity analyzer, structural anti-pattern scanner, and policy router",
			Run: func(ctx context.Context, args []string) error {
				if len(args) < 1 {
					printAnalyzeUsage()
					return nil
				}
				handleAnalyzeCommand(ctx, args[0], args[1:])
				return nil
			},
		}),
		legacyCmd(legacySpec{
			Name:        "vesicle",
			Aliases:     []string{"tokens", "token", "vesicle-quota", "quota", "tok"},
			Category:    "Terminal Boutons & Vesicles",
			Description: "Inspect 24h & 7d token consumption windows and tier quotas",
			Run: func(ctx context.Context, args []string) error {
				handleTokensCommand(ctx, args)
				return nil
			},
		}),
		legacyCmd(legacySpec{
			Name:        "saga",
			Aliases:     []string{"story", "stories", "synaptic-saga", "sty"},
			Category:    "Postsynaptic Effectors & Actuation",
			Description: "Autonomous multi-turn story execution and DAG branching",
			Run: func(ctx context.Context, args []string) error {
				if len(args) >= 1 && args[0] == "next" {
					handlePlaneCommand(ctx, "next", args[1:])
					return nil
				}
				if len(args) < 1 {
					printStoryUsage()
					return nil
				}
				handleStoryCommand(ctx, args[0], args[1:])
				return nil
			},
		}),
		legacyCmd(legacySpec{
			Name:        "quantal",
			Aliases:     []string{"commit", "quantal-commit", "cmt"},
			Category:    "Terminal Boutons & Vesicles",
			Description: "Autonomous Git commit with local model message generation",
			Run: func(ctx context.Context, args []string) error {
				handleCommitCommand(ctx, args)
				return nil
			},
		}),
		legacyCmd(legacySpec{
			Name:        "prune",
			Aliases:     []string{"audit-changes", "microglial-prune", "synaptic-pruning", "prn"},
			Category:    "Glial Regulation & Pruning",
			Description: "Topological vendor audit and rebase engine for local/* branches",
			Run: func(ctx context.Context, args []string) error {
				handleAuditChangesCommand(ctx, args)
				return nil
			},
		}),
		legacyCmd(legacySpec{
			Name:        "myelin",
			Aliases:     []string{"vcs", "hillock-guard", "myelin-guard", "guard"},
			Category:    "Soma & Axon Hillock",
			Description: "Git branch guardrails prohibiting main pushes and ensuring rebase integrity",
			Run: func(ctx context.Context, args []string) error {
				handleVCSCommand(ctx, args)
				return nil
			},
		}),
		legacyCmd(legacySpec{
			Name:        "github-sync",
			Aliases:     []string{"sync-github", "saltatory-sync", "salt-sync", "gsync"},
			Category:    "Axon & Myelin Sheath",
			Description: "Synchronize prioritized tickets and DAG dependencies to GitHub Issues",
			Run: func(ctx context.Context, args []string) error {
				handleGitHubSyncCommand(ctx, args)
				return nil
			},
		}),
		legacyCmd(legacySpec{
			Name:        "allostasis",
			Aliases:     []string{"quality", "homeostatic-audit", "allostasis-audit", "qlty"},
			Category:    "Glial Regulation & Pruning",
			Description: "Anti-mock scan, 100% coverage profiler, and HITL verification poller",
			Run: func(ctx context.Context, args []string) error {
				handleQualityCommand(ctx, args)
				return nil
			},
		}),
		legacyCmd(legacySpec{
			Name:        "sync-remote",
			Category:    "Axon & Myelin Sheath",
			Description: "Detect, fetch, and rebase remote fixes with commit reconciliation",
			Run: func(ctx context.Context, args []string) error {
				handleVCSCommand(ctx, []string{"sync-remote"})
				return nil
			},
		}),
		legacyCmd(legacySpec{
			Name:        "translocate",
			Aliases:     []string{"move", "axonal-translocation", "mv"},
			Category:    "Axon & Myelin Sheath",
			Description: "Zero-credential interactive service migration across SSH nodes",
			Run: func(ctx context.Context, args []string) error {
				handleMoveCommand(ctx, args)
				return nil
			},
		}),
		legacyCmd(legacySpec{
			Name:        "motor",
			Aliases:     []string{"service", "motor-unit", "effector-service", "svc"},
			Category:    "Postsynaptic Effectors & Actuation",
			Description: "Manage microservices (Plane, Penpot, Ollama, LiteLLM)",
			Run: func(ctx context.Context, args []string) error {
				if len(args) < 1 {
					printServiceUsage()
					return nil
				}
				handleServiceCommand(ctx, args[0], args[1:])
				return nil
			},
		}),
		legacyCmd(legacySpec{
			Name:        "canvas",
			Aliases:     []string{"creative-canvas", "interactive-canvas", "pcontext-canvas"},
			Category:    "Postsynaptic Effectors & Actuation",
			Description: "Interactive writing canvas and Penpot UI direct creative action bridge",
			Run: func(ctx context.Context, args []string) error {
				handleCanvasCommand(ctx, args)
				return nil
			},
		}),
		legacyCmd(legacySpec{
			Name:        "ask",
			Aliases:     []string{"query-project", "rag-ask", "q"},
			Category:    "Cortex & Sensory Integration",
			Description: "Ask questions to Dendrite about a project with local RAG and interactive paid fallback",
			Run: func(ctx context.Context, args []string) error {
				handleAskCommand(ctx, args)
				return nil
			},
		}),
		legacyCmd(legacySpec{
			Name:        "greeting",
			Aliases:     []string{"greet", "pcontext-greeting"},
			Category:    "Synaptic Cleft & Gating",
			Description: "Generate dynamic 10-style greeting with cliché suppression",
			Run: func(ctx context.Context, args []string) error {
				handleGreetingCommand(ctx, args)
				return nil
			},
		}),
	}
}
