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
	"Pick work from Plane with `dendrite plane next` (or `dendrite_ticket_smart_select`), never from `DEV_STATUS.md`.",
	"Every repository we can modify is a Plane project: run `dendrite plane project ensure` in it (it writes the committed `.dendrite/plane.json` that `dendrite plane`, `sync` and agents read). Repositories that depend on each other belong to the same synapse (`dendrite synapse create`). `dendrite plane project audit` lists any repository that breaks this rule.",
	"Need an isolated checkout? Run `dendrite work new <ticket>`. It creates a git worktree in `dendrite-work/` next to the main checkout. Never hand-make `git clone` or `git worktree add ../<dir>` copies: those mix with human projects and are never cleaned up.",
	"Commit with `dendrite commit` (it references the ticket and pushes to origin; never to main/master).",
	"Route every LLM call through Dendrite: the LiteLLM gateway (`http://localhost:4000`) for model access and `dendrite agent jobs submit --isolate` to delegate to Antigravity (AGY) or other models. Do not call provider APIs or spawn other agent CLIs directly.",
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
	sb.WriteString("> Managed by `dendrite env prepare`; edits here are overwritten. Change `pkg/env/bootstrap/workflow.go` instead.\n\n")
	sb.WriteString("Every agent session in this repository runs through Dendrite:\n\n")
	writeNumberedRules(&sb)
	sb.WriteString("\n## Projects and synapses\n\n")
	sb.WriteString("If you can modify a repository's code, it is a project: it has its own Plane project, and its `.dendrite/plane.json` (committed, with `.dendrite/project.id`) tells every Dendrite command which board to use. If it depends on or is used by another editable repository, both are members of one synapse.\n\n")
	sb.WriteString("```bash\n")
	sb.WriteString("dendrite plane project ensure [dir] [--identifier ID] [--import DEV_STATUS.md]   # adopt or create, canonical states, link\n")
	sb.WriteString("dendrite plane project show [dir]                     # which Plane project this repository uses\n")
	sb.WriteString("dendrite plane project audit [--roots dir,...]        # editable repos without a project, related repos without a shared synapse\n")
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
	sb.WriteString("## Delegating to other models\n\n")
	sb.WriteString("```bash\n")
	sb.WriteString("dendrite agent jobs submit --ticket DEN-123 --agent agy --scope pkg/foo --prompt-file prompt.md --isolate\n")
	sb.WriteString("```\n\n")
	sb.WriteString("`--isolate` runs the job in the ticket's `dendrite-work/` copy, so the delegated model never edits the orchestrator's checkout.\n")
	return sb.String()
}
