package bootstrap

import (
	"fmt"
	"strings"
)

// DendriteWorkflowSkill is the managed skill every agent family (Claude, Antigravity/AGY,
// Gemini) receives; it is rewritten on every prepare so the copies never drift.
const DendriteWorkflowSkill = "dendrite-workflow"

// skillDirs are the skill directories read by each supported agent family.
var skillDirs = []string{
	".agents/skills", // Antigravity (AGY) and AGENTS.md-aware tools
	".gemini/skills", // Gemini CLI
	".claude/skills", // Claude Code
}

// pointerFiles are per-agent entry files that import AGENTS.md. They are only
// created when missing, never overwritten, so hand-written content survives.
var pointerFiles = []string{"CLAUDE.md", "GEMINI.md"}

// pointerFileContent makes Claude Code and Gemini CLI load AGENTS.md.
const pointerFileContent = "@AGENTS.md\n"

// dendriteWorkflowRules is the single source of the Dendrite-first rules rendered into
// AGENTS.md, .agents/rules.md and the dendrite-workflow skill.
var dendriteWorkflowRules = []string{
	"Start every session with `dendrite session resume`; it also removes merged, clean working copies.",
	"Pick work from Plane with `dendrite chronos next` (alias `plane next`, or the `dendrite_ticket_smart_select` MCP tool), never from `DEV_STATUS.md`.",
	"Every change follows one ticket through its lifecycle, and a write lock enforces it: `dendrite hook pre-edit` refuses file changes in a working copy that has no claim. Pull the ticket (`dendrite chronos next`) or create it (`dendrite chronos create --name \"...\" --desc \"goal and acceptance criteria\"`), design the plan, then start it with `dendrite work new <ticket>`, which moves it to In Progress and claims it for the new copy (a claim by hand: `dendrite chronos claim <ticket>` or the `dendrite_ticket_claim` MCP tool; `dendrite_query_context` returns the ticket, its criteria and any saved plan). Each `dendrite quantal` commit and each `dendrite session checkpoint` leaves a comment on the ticket, and a plan you did not finish is stored on it (`dendrite chronos plan push`) so the next session resumes from it. When the work is finished run `dendrite chronos finish <ticket>` (MCP `dendrite_ticket_finish`): it runs the quality gate and checks the diff against each `[AC-n]`, then sets `done` when the project allows it (directive `[done] auto_done`) and the ticket has no `needs-human-review` label, `to verify` otherwise, and `blocked` with a handoff (completed, challenges, missing) when a check fails. To stop for another reason use `dendrite chronos handoff <ticket>` (blocked: write what it waits on and what is missing), `dendrite chronos ticket set <ticket> cancelled` when it is dropped or `backlog` when it is parked. A challenge found mid-plan is a new ticket linked with `dendrite chronos link`. Bypass the guards only deliberately, with `--no-ticket-guard`.",
	"Every repository we can modify is a Plane project: run `dendrite chronos project ensure` in it (it writes the local `.dendrite/plane.json` that `dendrite chronos`, `sync` and agents read; never commit anything under `.dendrite/`, and in a fresh clone this command recovers the project's identity). Repositories that depend on each other belong to the same synapse (`dendrite synapse create`). `dendrite chronos project audit` lists any repository that breaks this rule.",
	"Work on your own branch, never directly on `develop`, `main` or `master`: run `dendrite work new <ticket>` for each ticket. It creates a git worktree in `dendrite-work/` next to the main checkout, on branch `agent/<ticket>`. Never hand-make `git clone` or `git worktree add ../<dir>` copies: those mix with human projects and are never cleaned up.",
	"Commit with `dendrite quantal` (alias `commit`; it references the ticket and pushes your branch to origin, never to main/master).",
	"When the ticket is finished, merge your branch into `develop` and push, so everyone's work syncs through `develop`: from your copy run `git fetch origin && git merge origin/develop`, fix conflicts and re-run the tests on your branch, then `git push origin HEAD:develop`. If the push is rejected, someone else merged first: fetch, merge and push again. Never merge into or push `main`/`master`.",
	"Explore with Dendrite before any raw text search, cheapest first: `dendrite rag search`/`dendrite_rag_retrieve` (detail `map`, then `dendrite rag show`/`dendrite_rag_show` for the bodies you need), `dendrite rag symbols` for definitions and references, `dendrite graph` (`dendrite_graph_explain`, `dendrite_graph_path`) for callers and structure, `dendrite rag history` for past changes, and `dendrite ask` for questions. `dendrite rag grep` is the exact-pattern fallback (index-aware, capped). Use `rg`/`grep`/`find` only when those return nothing, and then scoped to a path with a result cap, because unscoped `rg` output costs far more tokens than a retrieval map.",
	"Questions about tickets (status, blockers, what a ticket is for) go to Plane through Dendrite: `dendrite ask --corpus tickets \"...\"` or `dendrite rag search --corpus tickets \"...\"` (MCP: `dendrite_rag_retrieve` with corpus `tickets`) for open questions, and `dendrite chronos`, `dendrite_plane_issues` or `dendrite_ticket_smart_select` for exact lookups. Never use `DEV_STATUS.md` or a search of the repository for this.",
	"Route every LLM call through Dendrite: the LiteLLM gateway (`http://localhost:4000`) for model access and `dendrite bouton jobs submit --isolate` (alias `agent jobs`) to delegate to Antigravity (AGY) or other models. Do not call provider APIs or spawn other agent CLIs directly.",
	"Run `dendrite session checkpoint` after each finished ticket and before long waits.",
	"When a copy is finished and merged, `dendrite work gc` (or the next resume/checkpoint) deletes it; `dendrite work done <name>` removes one explicitly.",
}

func writeNumberedRules(sb *strings.Builder) {
	for i, rule := range dendriteWorkflowRules {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, rule))
	}
}

// renderDendriteWorkflowSkill renders the managed dendrite-workflow SKILL.md.
func renderDendriteWorkflowSkill() string {
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString("name: " + DendriteWorkflowSkill + "\n")
	sb.WriteString("description: Dendrite-first workflow for every model (Claude, Antigravity/AGY, Gemini, Copilot). Use before starting a session, creating a branch, clone or worktree, committing, or delegating work to another model.\n")
	sb.WriteString("---\n\n")
	sb.WriteString("# Dendrite-First Workflow\n\n")
	sb.WriteString("> Managed by `dendrite soma prepare`; edits here are overwritten. Change `pkg/env/bootstrap/workflow.go` instead.\n\n")
	sb.WriteString("Every agent session in this repository runs through Dendrite. Commands use Neuro-Lore names; the plain names (`env`, `plane`, `agent`, `commit`, ...) still work as aliases, and `dendrite <command> --help` shows every flag.\n\n")
	writeNumberedRules(&sb)
	sb.WriteString("\n## Projects and synapses\n\n")
	sb.WriteString("If you can modify a repository's code, it is a project: it has its own Plane project, and its `.dendrite/plane.json` (local and untracked, like all of `.dendrite/`, together with `.dendrite/project.id`) tells every Dendrite command which board to use. If it depends on or is used by another editable repository, both are members of one synapse.\n\n")
	sb.WriteString("```bash\n")
	sb.WriteString("dendrite chronos project ensure [dir] [--identifier ID] [--import DEV_STATUS.md] # adopt or create, canonical states, link\n")
	sb.WriteString("dendrite chronos project show [dir]                   # which Plane project this repository uses\n")
	sb.WriteString("dendrite chronos project audit [--roots dir,...]      # editable repos without a project, related repos without a shared synapse\n")
	sb.WriteString("dendrite synapse create <id> --members <dir>=alias:role,...   # link related repositories\n")
	sb.WriteString("```\n\n")
	sb.WriteString("A repository whose origin belongs to someone else (a third-party clone) is not editable and needs no project.\n")
	sb.WriteString("\n## Working copies\n\n")
	sb.WriteString("```bash\n")
	sb.WriteString("dendrite work new DEN-123          # ../dendrite-work/<repo>-DEN-123 on branch agent/den-123\n")
	sb.WriteString("dendrite work list                 # status: dirty, unpushed, merged, upstream gone\n")
	sb.WriteString("dendrite work done DEN-123         # remove one copy (refuses unsaved work without --force)\n")
	sb.WriteString("dendrite work gc [--dry-run]       # remove merged + clean copies\n")
	sb.WriteString("dendrite work adopt [--dry-run]    # move legacy sibling worktrees into dendrite-work/\n")
	sb.WriteString("```\n\n")
	sb.WriteString("A copy is deleted automatically only when its branch is merged into main/master/develop (or its upstream branch was deleted) **and** it has no uncommitted or unpushed work. Copies used by in-flight agent jobs, the current directory, and story-fork variants are never removed.\n\n")
	sb.WriteString("## Finishing a ticket\n\n")
	sb.WriteString("Each agent works on its own branch and merges it into `develop` when the ticket is done; `develop` is where everyone's work meets. From your `dendrite-work/` copy (where `develop` cannot be checked out, because the main checkout holds it):\n\n")
	sb.WriteString("```bash\n")
	sb.WriteString("dendrite quantal --ticket DEN-123        # commit and push agent/den-123\n")
	sb.WriteString("git fetch origin\n")
	sb.WriteString("git merge origin/develop                 # take in everyone else's work; fix conflicts, re-run the tests\n")
	sb.WriteString("git push origin HEAD:develop             # fast-forward develop to your branch\n")
	sb.WriteString("dendrite session checkpoint              # record it; the merged copy is cleaned up later\n")
	sb.WriteString("```\n\n")
	sb.WriteString("A rejected push means `develop` moved since your fetch: fetch, merge and push again. Never merge into or push `main`/`master`.\n\n")
	sb.WriteString("## Delegating to other models\n\n")
	sb.WriteString("```bash\n")
	sb.WriteString("dendrite bouton jobs submit --ticket DEN-123 --agent agy --scope pkg/foo --prompt-file prompt.md --isolate\n")
	sb.WriteString("dendrite bouton jobs wait --ticket DEN-123   # block until the job finishes\n")
	sb.WriteString("```\n\n")
	sb.WriteString("`--isolate` runs the job in the ticket's `dendrite-work/` copy, so the delegated model never edits the orchestrator's checkout.\n\n")
	sb.WriteString("### What to delegate\n\n")
	sb.WriteString("Delegate work that is mechanical and checkable: updating test expectations after a deliberate behavior change, fixtures and golden files, docs and seed or config edits, boilerplate from a clear spec, first drafts of modules whose protocol you already fixed. Use `--agent agy` (complexity 1-3 picks a fast model) or a local Ollama model for the cheapest tier. Keep design, protocols, concurrency, ranking and debugging with the strong model, and review everything a delegate returns.\n\n")
	sb.WriteString("### How to delegate well\n\n")
	sb.WriteString("- Give the job its own ticket's copy (`--ticket`, `--worktree` or `--isolate`) and a tight `--scope` of the paths it may change; never edit those files yourself while it runs.\n")
	sb.WriteString("- Write the acceptance in the prompt: what must pass (`go test ./pkg/x`), what must not be weakened (no loosened assertions), and that it must not commit.\n")
	sb.WriteString("- `dendrite bouton jobs wait --ticket DEN-123`, then read the diff (`git diff`), re-run the tests yourself, and only then commit with `dendrite quantal`.\n")
	sb.WriteString("- Tests use real local servers and binaries. The zero-mock gate rejects `fake*`/`mock*`/`stub*` identifiers and `t.Skip`; an opt-in suite goes behind a build tag instead.\n\n")
	sb.WriteString("## Ending the plan\n\n")
	sb.WriteString("When the plan ends, record where the ticket stands before moving on: `dendrite chronos ticket set DEN-123 \"to verify\"` when it is finished, `blocked` when it waits on something (say what in the ticket), `cancelled` when it is dropped, `backlog` when it is parked. Agents never set `done`; a human signs off from To Verify.\n")
	return sb.String()
}
