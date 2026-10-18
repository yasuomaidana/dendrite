package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"Dendrite/pkg/agent"
	"Dendrite/pkg/agentjobs"
	"Dendrite/pkg/agentsearch"
)

func handleAgentSearchCommand(ctx context.Context, args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: dendrite agent search <query> [--pkg <package>] [--local] [--max-cost <usd>]")
		return
	}

	var queryParts []string
	var targetPkg string
	var requireLocal bool
	var maxCost float64

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--pkg", "--package":
			if i+1 < len(args) {
				targetPkg = args[i+1]
				i++
			}
		case "--local":
			requireLocal = true
		case "--max-cost":
			if i+1 < len(args) {
				maxCost, _ = strconv.ParseFloat(args[i+1], 64)
				i++
			}
		default:
			queryParts = append(queryParts, args[i])
		}
	}

	qStr := strings.TrimSpace(strings.Join(queryParts, " "))
	if qStr == "" {
		fmt.Println("[-] Please provide a search query")
		return
	}

	q := agentsearch.NewQueryBuilder().
		WithText(qStr).
		WithTargetPackage(targetPkg).
		RequireLocalInference(requireLocal).
		WithMaxCost(maxCost).
		Build()

	engine := agentsearch.NewSearchEngine(nil, nil, nil)
	results, err := engine.Search(ctx, q)
	if err != nil {
		fmt.Printf("[-] Agent search error: %v\n", err)
		return
	}

	fmt.Printf("[+] Agent Search Results (%d matching candidates):\n\n", len(results))
	for i, res := range results {
		fmt.Printf("  %d. [%s] %s\n", i+1, res.Manifest.ID, res.Manifest.Name)
		fmt.Printf("     - Capability Score: %.2f (RRF: %.2f, Dense: %.2f, Sparse: %.2f)\n",
			res.FinalScore, res.RRFScore, res.DenseScore, res.SparseScore)
		fmt.Printf("     - Preferred Tier:   %s (Engine: %s, Model: %s)\n",
			res.Manifest.PreferredTier, res.Manifest.PreferredEngine, res.Manifest.PreferredModel)
		fmt.Printf("     - Zero-Cost:        %v\n", res.Manifest.ZeroCostCandidate)
		fmt.Printf("     - Match Reasoning:  %s\n\n", res.Reasoning)
	}
}

func handleAgentRouteCommand(ctx context.Context, args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: dendrite agent route --task <desc> [--pkg <package>] [--exec] [--local]")
		return
	}

	var taskDesc string
	var targetPkg string
	var execute bool
	var requireLocal bool

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--task", "--prompt":
			if i+1 < len(args) {
				taskDesc = args[i+1]
				i++
			}
		case "--pkg", "--package":
			if i+1 < len(args) {
				targetPkg = args[i+1]
				i++
			}
		case "--exec", "--execute":
			execute = true
		case "--local":
			requireLocal = true
		default:
			if taskDesc == "" && !strings.HasPrefix(args[i], "--") {
				taskDesc = args[i]
			}
		}
	}

	taskDesc = strings.TrimSpace(taskDesc)
	if taskDesc == "" {
		fmt.Println("[-] Please provide a task description via --task \"...\"")
		return
	}

	q := agentsearch.NewQueryBuilder().
		WithText(taskDesc).
		WithTargetPackage(targetPkg).
		RequireLocalInference(requireLocal).
		Build()

	engine := agentsearch.NewSearchEngine(nil, nil, nil)
	decision, err := engine.Route(ctx, q)
	if err != nil {
		fmt.Printf("[-] Routing error: %v\n", err)
		return
	}

	fmt.Printf("[+] Optimal Agent Routing Decision:\n")
	fmt.Printf("    - Selected Agent:   %s (%s)\n", decision.SelectedAgent.ID, decision.SelectedAgent.Name)
	fmt.Printf("    - Capability Score:  %.2f\n", decision.Score)
	fmt.Printf("    - Recommended Tier:  %s\n", decision.Tier)
	fmt.Printf("    - Target Engine:    %s\n", decision.EngineID)
	fmt.Printf("    - Target Model:     %s\n", decision.Model)
	fmt.Printf("    - Virtual Key:      %s\n", decision.VirtualKeyAlias)
	fmt.Printf("    - Estimated Cost:   $%.2f\n", decision.EstimatedCost)
	fmt.Printf("    - Routing Rationale: %s\n", decision.Reasoning)

	if execute {
		fmt.Printf("\n[+] Auto-launching agent harness (%s)...\n", decision.SelectedAgent.ID)
		cwd, _ := os.Getwd()
		cfg := agent.AgentConfig{
			Type:            decision.SelectedAgent.Type,
			LiteLLMBaseURL:  "http://localhost:4000/v1",
			VirtualAPIKey:   decision.VirtualKeyAlias,
			Model:           decision.Model,
			WorkspaceDir:    cwd,
			TaskDescription: taskDesc,
		}
		var runner agent.Runner
		if requireLocal {
			runner = agent.NewSandboxedRunner(nil, nil)
		} else {
			runner = agent.NewHostRunner(true)
		}
		summary, rErr := runner.Execute(ctx, cfg)
		if rErr != nil {
			fmt.Printf("[-] Agent execution failed: %v\n", rErr)
			return
		}
		fmt.Printf("[✓] Executed successfully (Session ID: %s, Duration: %v)\n", summary.SessionID, summary.Duration)
	}
}

func init() {
	if len(os.Args) >= 3 && isAgentJobsInvocation(os.Args[1], os.Args[2]) {
		ctx := context.Background()
		if err := handleAgentJobsCommand(ctx, os.Args[3:]); err != nil {
			fmt.Fprintf(os.Stderr, "[-] %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}
}

func isAgentJobsInvocation(cmd, sub string) bool {
	if strings.ToLower(sub) != "jobs" {
		return false
	}
	switch strings.ToLower(cmd) {
	case "agent", "effector-dispatch", "bouton-dispatch", "dispatch", "bouton", "agt":
		return true
	default:
		return false
	}
}

func handleAgentJobsCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		printAgentJobsUsage()
		return nil
	}

	switch args[0] {
	case "submit":
		return handleAgentJobsSubmit(ctx, args[1:])
	case "list":
		return handleAgentJobsList(ctx, args[1:])
	case "show":
		return handleAgentJobsShow(ctx, args[1:])
	case "tail":
		return handleAgentJobsTail(ctx, args[1:])
	case "wait":
		return handleAgentJobsWait(ctx, args[1:])
	case "retry":
		return handleAgentJobsRetry(ctx, args[1:])
	case "cancel":
		return handleAgentJobsCancel(ctx, args[1:])
	case "daemon":
		return handleAgentJobsDaemon(ctx, args[1:])
	case "run":
		return handleAgentJobsRun(ctx, args[1:])
	default:
		return fmt.Errorf("unknown agent jobs action: %s", args[0])
	}
}

func printAgentJobsUsage() {
	fmt.Println("Usage: dendrite agent jobs <submit|list|show|tail|wait|retry|cancel|daemon|run> [flags...]")
	fmt.Println("\nSubcommands:")
	fmt.Println("  submit   --ticket <T> --agent <A> --model <M> --scope <S> --prompt-file <P> [--worktree <W>] [--complexity <C>]")
	fmt.Println("  list     [--json]")
	fmt.Println("  show     <id>")
	fmt.Println("  tail     <id>")
	fmt.Println("  wait     [--ticket <T>] [--timeout <duration>]")
	fmt.Println("  retry    <id>")
	fmt.Println("  cancel   <id>")
	fmt.Println("  daemon   [--once] [--interval <duration>] [--install]")
	fmt.Println("  run      <id> (internal)")
}

func handleAgentJobsSubmit(ctx context.Context, args []string) error {
	var (
		ticket     string
		agentName  = "agy"
		model      string
		scopes     []string
		promptFile string
		promptText string
		worktree   string
		complexity = 1
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--ticket":
			if i+1 < len(args) {
				ticket = args[i+1]
				i++
			}
		case "--agent":
			if i+1 < len(args) {
				agentName = args[i+1]
				i++
			}
		case "--model":
			if i+1 < len(args) {
				model = args[i+1]
				i++
			}
		case "--scope":
			if i+1 < len(args) {
				raw := args[i+1]
				for _, s := range strings.Split(raw, ",") {
					if trimmed := strings.TrimSpace(s); trimmed != "" {
						scopes = append(scopes, trimmed)
					}
				}
				i++
			}
		case "--prompt-file":
			if i+1 < len(args) {
				promptFile = args[i+1]
				i++
			}
		case "--prompt":
			if i+1 < len(args) {
				promptText = args[i+1]
				i++
			}
		case "--worktree":
			if i+1 < len(args) {
				worktree = args[i+1]
				i++
			}
		case "--complexity":
			if i+1 < len(args) {
				if c, err := strconv.Atoi(args[i+1]); err == nil {
					complexity = c
				}
				i++
			}
		}
	}

	if ticket == "" {
		return errors.New("missing required flag --ticket")
	}
	if promptFile == "" && promptText == "" {
		return errors.New("missing required flag --prompt-file or --prompt")
	}

	spec := agentjobs.SubmitSpec{
		Ticket:     ticket,
		Agent:      agentName,
		Model:      model,
		Scope:      scopes,
		Worktree:   worktree,
		PromptFile: promptFile,
		Prompt:     promptText,
		Complexity: complexity,
	}

	id, err := agentjobs.Submit(spec)
	if err != nil {
		return fmt.Errorf("failed to submit job: %w", err)
	}

	fmt.Printf("[✓] Job submitted successfully: %s\n", id)
	return nil
}

func handleAgentJobsList(ctx context.Context, args []string) error {
	asJSON := false
	for _, a := range args {
		if a == "--json" {
			asJSON = true
		}
	}

	l := agentjobs.NewLedger("")
	jobs, err := l.List()
	if err != nil {
		return fmt.Errorf("failed to list jobs: %w", err)
	}

	if asJSON {
		data, err := json.MarshalIndent(jobs, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("[+] Agent Jobs (%d):\n", len(jobs))
	if len(jobs) == 0 {
		fmt.Println("    No jobs recorded.")
		return nil
	}
	fmt.Printf("  %-38s %-10s %-8s %-16s %s\n", "JOB ID", "TICKET", "AGENT", "STATUS", "UPDATED")
	for _, j := range jobs {
		fmt.Printf("  %-38s %-10s %-8s %-16s %s\n", j.ID, j.Ticket, j.Agent, j.Status, j.UpdatedAt.Format(time.RFC3339))
	}
	return nil
}

func handleAgentJobsShow(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: dendrite agent jobs show <id>")
	}
	id := args[0]
	l := agentjobs.NewLedger("")
	job, err := l.Load(id)
	if err != nil {
		return fmt.Errorf("failed to load job %q: %w", id, err)
	}
	events, _ := l.Events(id)

	fmt.Printf("[+] Job Specification: %s\n", job.ID)
	fmt.Printf("    Ticket:          %s\n", job.Ticket)
	fmt.Printf("    Status:          %s\n", job.Status)
	fmt.Printf("    Agent:           %s\n", job.Agent)
	fmt.Printf("    Model:           %s\n", job.Model)
	fmt.Printf("    Complexity:      %d\n", job.Complexity)
	fmt.Printf("    Worktree:        %s\n", job.Worktree)
	fmt.Printf("    Scope:           %s\n", strings.Join(job.Scope, ", "))
	fmt.Printf("    Prompt File:     %s\n", job.PromptFile)
	if job.ConversationID != "" {
		fmt.Printf("    Conversation ID: %s\n", job.ConversationID)
	}
	if job.PGID > 0 {
		fmt.Printf("    Process Group:   %d\n", job.PGID)
	}
	fmt.Printf("    Created At:      %s\n", job.CreatedAt.Format(time.RFC3339))
	fmt.Printf("    Updated At:      %s\n", job.UpdatedAt.Format(time.RFC3339))

	if len(events) > 0 {
		fmt.Printf("\n[+] Events History (%d):\n", len(events))
		for _, ev := range events {
			dataBytes, _ := json.Marshal(ev.Data)
			fmt.Printf("    %d. [%s] %s %s\n", ev.Seq, ev.Time.Format(time.RFC3339), ev.Type, string(dataBytes))
		}
	}
	return nil
}

func handleAgentJobsTail(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: dendrite agent jobs tail <id>")
	}
	id := args[0]
	l := agentjobs.NewLedger("")
	logPath := l.AgentLogPath(id)
	data, err := os.ReadFile(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("[-] No agent.log found for job %s\n", id)
			return nil
		}
		return fmt.Errorf("failed to read agent.log: %w", err)
	}

	lines := strings.Split(strings.TrimRight(string(data), "\r\n"), "\n")
	if len(lines) > 50 {
		lines = lines[len(lines)-50:]
	}
	for _, l := range lines {
		fmt.Println(l)
	}
	return nil
}

func handleAgentJobsWait(ctx context.Context, args []string) error {
	ticketFilter := ""
	timeout := 30 * time.Minute
	interval := 2 * time.Second

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--ticket":
			if i+1 < len(args) {
				ticketFilter = args[i+1]
				i++
			}
		case "--timeout":
			if i+1 < len(args) {
				if d, err := time.ParseDuration(args[i+1]); err == nil {
					timeout = d
				}
				i++
			}
		case "--interval":
			if i+1 < len(args) {
				if d, err := time.ParseDuration(args[i+1]); err == nil {
					interval = d
				}
				i++
			}
		}
	}

	l := agentjobs.NewLedger("")
	allJobs, err := l.List()
	if err != nil {
		return fmt.Errorf("failed to list jobs: %w", err)
	}

	initialDone := make(map[string]time.Time)
	var targetJobs []*agentjobs.Job
	for _, j := range allJobs {
		if ticketFilter == "" || j.Ticket == ticketFilter {
			targetJobs = append(targetJobs, j)
			if fi, err := os.Stat(l.DoneMarkerPath(j.ID)); err == nil {
				initialDone[j.ID] = fi.ModTime()
			}
		}
	}

	if len(targetJobs) == 0 {
		fmt.Println("[-] No matching jobs found")
		return nil
	}

	allAlreadyDone := true
	for _, j := range targetJobs {
		if _, ok := initialDone[j.ID]; !ok {
			allAlreadyDone = false
			break
		}
	}
	if allAlreadyDone {
		for _, j := range targetJobs {
			fmt.Printf("[✓] Job finished: %s\n", j.ID)
		}
		return nil
	}

	deadline := time.Now().Add(timeout)
	finished := make(map[string]bool)

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("wait timed out after %v", timeout)
		}

		time.Sleep(interval)

		currentJobs, err := l.List()
		if err == nil {
			for _, j := range currentJobs {
				if ticketFilter != "" && j.Ticket != ticketFilter {
					continue
				}
				if finished[j.ID] {
					continue
				}
				markerPath := l.DoneMarkerPath(j.ID)
				fi, err := os.Stat(markerPath)
				if err != nil {
					continue
				}
				prevTime, hadInitial := initialDone[j.ID]
				if !hadInitial || fi.ModTime().After(prevTime) {
					finished[j.ID] = true
					fmt.Printf("[✓] Job finished: %s\n", j.ID)
				}
			}
		}

		allFinished := true
		for _, j := range targetJobs {
			if _, ok := initialDone[j.ID]; !ok && !finished[j.ID] {
				allFinished = false
				break
			}
		}
		if allFinished && len(finished) > 0 {
			return nil
		}
	}
}

func handleAgentJobsRetry(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: dendrite agent jobs retry <id>")
	}
	id := args[0]
	d := agentjobs.DefaultDispatcher
	if err := d.Retry(id); err != nil {
		return err
	}
	fmt.Printf("[✓] Job %s re-dispatched successfully\n", id)
	return nil
}

func handleAgentJobsCancel(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: dendrite agent jobs cancel <id>")
	}
	id := args[0]
	d := agentjobs.DefaultDispatcher
	if err := d.Cancel(id); err != nil {
		return err
	}
	fmt.Printf("[✓] Job %s cancelled successfully\n", id)
	return nil
}

func handleAgentJobsRun(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: dendrite agent jobs run <id>")
	}
	id := args[0]
	return agentjobs.RunJob(ctx, id)
}

func handleAgentJobsDaemon(ctx context.Context, args []string) error {
	var (
		once     bool
		install  bool
		interval = agentjobs.DefaultDaemonInterval
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--once":
			once = true
		case "--install":
			install = true
		case "--interval":
			if i+1 < len(args) {
				if d, err := time.ParseDuration(args[i+1]); err == nil {
					interval = d
				}
				i++
			}
		}
	}

	if install {
		plistPath, err := agentjobs.InstallLaunchAgent("", "")
		if err != nil {
			return fmt.Errorf("failed to install launch agent: %w", err)
		}
		fmt.Printf("[✓] LaunchAgent installed to %s\n", plistPath)
		return nil
	}

	w := agentjobs.NewWatcher(nil, nil)
	if once {
		return w.Tick(ctx)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Initial run
	if err := w.Tick(ctx); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintf(os.Stderr, "[-] daemon tick error: %v\n", err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := w.Tick(ctx); err != nil && !errors.Is(err, context.Canceled) {
				fmt.Fprintf(os.Stderr, "[-] daemon tick error: %v\n", err)
			}
		}
	}
}
