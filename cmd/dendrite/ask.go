package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"Dendrite/pkg/rag"
)

func handleAskCommand(ctx context.Context, args []string) {
	if len(args) == 0 {
		printAskUsage()
		return
	}

	var questionParts []string
	projectPath := "."
	autoApprove := false
	forcePaid := false
	hybridMode := false
	updateIndex := false
	serverURL := ""
	paidModel := "gemini-2.5-pro"
	localModel := ""
	jsonOutput := false
	topK := 4

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-y", "--yes", "--auto-paid":
			autoApprove = true
		case "--paid", "--force-paid", "--cloud", "--use-paid":
			forcePaid = true
		case "--hybrid", "--local-first-refine":
			hybridMode = true
		case "--update-index", "--refresh", "--reindex":
			updateIndex = true
		case "--server", "--central":
			if i+1 < len(args) {
				serverURL = args[i+1]
				i++
			}
		case "--tier":
			if i+1 < len(args) {
				val := strings.ToLower(args[i+1])
				if val == "paid" || val == "cloud" {
					forcePaid = true
				} else if val == "hybrid" {
					hybridMode = true
				}
				i++
			}
		case "-p", "--project", "--dir":
			if i+1 < len(args) {
				projectPath = args[i+1]
				i++
			}
		case "--paid-model", "--cloud-model":
			if i+1 < len(args) {
				paidModel = args[i+1]
				i++
			}
		case "--local-model":
			if i+1 < len(args) {
				localModel = args[i+1]
				i++
			}
		case "--top-k", "-k":
			if i+1 < len(args) {
				if v, err := strconv.Atoi(args[i+1]); err == nil {
					topK = v
				}
				i++
			}
		case "--json":
			jsonOutput = true
		case "-h", "--help", "help":
			printAskUsage()
			return
		default:
			if !strings.HasPrefix(arg, "-") {
				questionParts = append(questionParts, arg)
			}
		}
	}

	question := strings.TrimSpace(strings.Join(questionParts, " "))
	if question == "" {
		fmt.Println("[-] Error: question cannot be empty.")
		printAskUsage()
		return
	}

	engine := rag.NewAskEngine(nil)

	req := rag.AskRequest{
		Question:        question,
		ProjectPath:     projectPath,
		AutoApprovePaid: autoApprove,
		ForcePaid:       forcePaid,
		HybridMode:      hybridMode,
		UpdateIndex:     updateIndex,
		ServerURL:       serverURL,
		PaidModel:       paidModel,
		LocalModel:      localModel,
		TopK:            topK,
		Input:           os.Stdin,
		Output:          os.Stdout,
	}

	if serverURL != "" {
		fmt.Printf("[+] Forwarding query to Central RAG Service at %s for: %q\n", serverURL, question)
	} else {
		fmt.Printf("[+] Querying Dendrite RAG index for: %q\n", question)
	}

	res, err := engine.Ask(ctx, req)
	if err != nil {
		fmt.Printf("[-] Query failed: %v\n", err)
		return
	}

	if jsonOutput {
		data, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			fmt.Printf("[-] Failed to serialize JSON output: %v\n", err)
			return
		}
		fmt.Println(string(data))
		return
	}

	tierDisplay := "Zero-Cost Local LLM"
	switch res.SourceTier {
	case "paid-cloud":
		tierDisplay = "Paid Cloud LLM (Escalated)"
	case "hybrid":
		tierDisplay = "Hybrid Mode (Local First + Paid Cloud Refinement)"
	case "central-rag":
		tierDisplay = "Central Shared RAG Service"
	}

	cacheNote := ""
	if res.IndexStatus != "" {
		cacheNote = fmt.Sprintf(" | Index: %s", res.IndexStatus)
	}

	fmt.Println("\n================================================================================")
	fmt.Printf("[✓] Answer (%s | Model: %s | Time: %v%s):\n", tierDisplay, res.Model, res.Duration.Round(10*time.Millisecond), cacheNote)
	fmt.Println("================================================================================")
	fmt.Println(res.Answer)
	fmt.Println("================================================================================")

	if res.LocalDraft != "" && res.SourceTier == "hybrid" {
		fmt.Println("\n[+] Note: Initial draft was synthesized locally at zero cost, then verified and refined via cloud LLM.")
	}

	if len(res.RetrievedChunks) > 0 {
		fmt.Println("\n[+] Retrieved Project Documentation & Code Context:")
		for i, c := range res.RetrievedChunks {
			heading := ""
			if c.Heading != "" {
				heading = fmt.Sprintf(" > %s", c.Heading)
			}
			fmt.Printf("    %d. %s (lines %d-%d)%s\n", i+1, c.FilePath, c.LineStart, c.LineEnd, heading)
		}
	}
}

func printAskUsage() {
	fmt.Println("Usage: dendrite ask [flags] \"<question>\"")
	fmt.Println("\nAsk questions about the project using local RAG retrieval with hybrid refinement or paid cloud fallback.")
	fmt.Println("\nOptions:")
	fmt.Println("  --hybrid           Hybrid mode: draft locally at zero cost, then refine & verify with paid cloud model")
	fmt.Println("  -y, --yes          Automatically escalate to paid cloud model if local LLM fails without interactive prompt")
	fmt.Println("  --paid, --force-paid Bypass local LLM and directly query the paid cloud model with RAG context")
	fmt.Println("  --server <url>     Delegate RAG query to a centralized remote Dendrite service (e.g. http://host:3456)")
	fmt.Println("  --update-index     Force cache invalidation and re-index project files incrementally")
	fmt.Println("  -p, --project <dir>Target project directory to index and search (default: \".\")")
	fmt.Println("  --paid-model <name>Paid cloud model fallback (default: \"gemini-2.5-pro\")")
	fmt.Println("  --local-model <m>  Preferred local model (default: auto-detected from Ollama/OxiLLaMa)")
	fmt.Println("  -k, --top-k <num>  Number of context chunks to retrieve (default: 4)")
	fmt.Println("  --json             Emit structured JSON response")
	fmt.Println("\nExamples:")
	fmt.Println("  dendrite ask \"I want to know how to check available LLMs\"")
	fmt.Println("  dendrite ask --hybrid \"I want to know how to check available LLMs\"")
	fmt.Println("  dendrite ask -y \"I want to know how to check available LLMs\"")
	fmt.Println("  dendrite ask --paid \"I want to know how to check available LLMs\" (bypasses local LLM)")
	fmt.Println("  dendrite ask --server http://100.84.12.10:3456 \"How does the token gateway work?\"")
	fmt.Println("  dendrite ask --update-index \"Where is the telemetry pipeline defined?\"")
}
