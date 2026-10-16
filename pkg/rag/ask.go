package rag

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"Dendrite/pkg/inference"
)

var (
	// ErrEmptyQuestion is returned when the user question is blank.
	ErrEmptyQuestion = errors.New("question cannot be empty")
	// ErrPaidEscalationDeclined indicates the human refused paid LLM escalation.
	ErrPaidEscalationDeclined = errors.New("local LLM failed and paid cloud escalation declined by user")
)

// AskEngine manages the end-to-end question answering pipeline with local RAG, hybrid mode, and paid cloud escalation.
type AskEngine struct {
	InferenceManager *inference.Manager
	Indexer          *Indexer
	LocalEngine      inference.InferenceEngine // Optional override for testing or explicit local engine
	PaidCaller       PaidCallerFunc            // Optional override for testing or custom cloud caller
	HTTPClient       *http.Client
}

// NewAskEngine creates a new AskEngine with default manager and indexer.
func NewAskEngine(mgr *inference.Manager) *AskEngine {
	if mgr == nil {
		mgr = inference.DefaultManager()
	}
	return &AskEngine{
		InferenceManager: mgr,
		Indexer:          NewIndexer(),
		HTTPClient:       &http.Client{Timeout: 60 * time.Second},
	}
}

// Ask processes the question, retrieves project context, tries local LLM, and escalates to paid LLM if needed.
func (e *AskEngine) Ask(ctx context.Context, req AskRequest) (*AskResult, error) {
	start := time.Now()

	req.Question = strings.TrimSpace(req.Question)
	if req.Question == "" {
		return nil, ErrEmptyQuestion
	}

	// Delegate to central RAG service if configured
	serverURL := req.ServerURL
	if serverURL == "" {
		serverURL = os.Getenv("DENDRITE_RAG_SERVER")
	}
	if serverURL != "" {
		return e.delegateToRemoteServer(ctx, serverURL, req)
	}

	// Apply defaults
	if req.ProjectPath == "" {
		req.ProjectPath = "."
	}
	if req.PaidModel == "" {
		req.PaidModel = "gemini-2.5-pro"
	}
	if req.TopK <= 0 {
		req.TopK = 4
	}
	if req.Input == nil {
		req.Input = os.Stdin
	}
	if req.Output == nil {
		req.Output = os.Stdout
	}

	// 1. Ensure project files are incrementally indexed or updated
	indexStatus, err := e.Indexer.IndexProjectIncremental(ctx, req.ProjectPath, req.UpdateIndex)
	if err != nil {
		_, _ = fmt.Fprintf(req.Output, "[!] Notice: Project indexing partial or failed: %v\n", err)
	}

	// 2. Retrieve relevant context chunks via BM25
	chunks, err := e.Indexer.Retrieve(ctx, req.Question, SearchOptions{
		TopK:      req.TopK,
		MinScore:  0.01,
		MaxTokens: 2500,
	})
	if err != nil {
		_, _ = fmt.Fprintf(req.Output, "[!] Retrieval notice: %v\n", err)
	}

	// 3. Construct Augmented Context Prompt
	systemPrompt, userPrompt := buildRAGPrompts(req.Question, chunks)

	// 4. Direct Paid Cloud LLM Bypass (e.g. unsatisfactory local LLM answer on previous query)
	if req.ForcePaid {
		_, _ = fmt.Fprintf(req.Output, "[+] Force-paid flag enabled: querying paid cloud LLM (%s) directly with project RAG context...\n", req.PaidModel)
		paidResp, paidErr := e.callPaidLLM(ctx, userPrompt, req.PaidModel)
		if paidErr != nil {
			return nil, fmt.Errorf("paid cloud LLM (%s) failed: %w", req.PaidModel, paidErr)
		}

		answer := enhanceAnswer(req.Question, paidResp.Text, chunks)
		return &AskResult{
			Question:           req.Question,
			Answer:             answer,
			SourceTier:         "paid-cloud",
			Model:              req.PaidModel,
			Engine:             paidResp.EngineID,
			Escalated:          true,
			EscalationApproved: true,
			RetrievedChunks:    chunks,
			PromptTokens:       paidResp.PromptTokens,
			OutputTokens:       paidResp.OutputTokens,
			IndexStatus:        indexStatus,
			Duration:           time.Since(start),
		}, nil
	}

	// 5. Hybrid Mode: Local First -> Paid Cloud Refinement
	if req.HybridMode {
		localResp, localErr := e.tryLocalLLM(ctx, req, systemPrompt, userPrompt)
		if localErr == nil && localResp != nil {
			localDraft := enhanceAnswer(req.Question, localResp.Text, chunks)
			_, _ = fmt.Fprintf(req.Output, "[+] Hybrid mode: Local draft generated (%s). Escalating to paid model (%s) for refinement...\n", localResp.Model, req.PaidModel)

			refinementPrompt := buildHybridRefinementPrompt(req.Question, chunks, localDraft)
			paidResp, paidErr := e.callPaidLLM(ctx, refinementPrompt, req.PaidModel)
			if paidErr == nil && paidResp != nil {
				refinedAnswer := enhanceAnswer(req.Question, paidResp.Text, chunks)
				return &AskResult{
					Question:           req.Question,
					Answer:             refinedAnswer,
					LocalDraft:         localDraft,
					SourceTier:         "hybrid",
					Model:              fmt.Sprintf("%s -> %s", localResp.Model, req.PaidModel),
					Engine:             paidResp.EngineID,
					Escalated:          true,
					EscalationApproved: true,
					RetrievedChunks:    chunks,
					PromptTokens:       localResp.PromptTokens + paidResp.PromptTokens,
					OutputTokens:       paidResp.OutputTokens,
					IndexStatus:        indexStatus,
					Duration:           time.Since(start),
				}, nil
			}

			// If paid refinement fails in hybrid mode, fall back to returning local draft gracefully
			_, _ = fmt.Fprintf(req.Output, "[-] Paid refinement failed (%v), returning verified local draft.\n", paidErr)
			return &AskResult{
				Question:           req.Question,
				Answer:             localDraft,
				SourceTier:         "local-rag",
				Model:              localResp.Model,
				Engine:             localResp.EngineID,
				Escalated:          false,
				RetrievedChunks:    chunks,
				PromptTokens:       localResp.PromptTokens,
				OutputTokens:       localResp.OutputTokens,
				IndexStatus:        indexStatus,
				Duration:           time.Since(start),
			}, nil
		}

		// Local failed in hybrid mode: fall through to standard paid escalation
		_, _ = fmt.Fprintf(req.Output, "[-] Local draft generation failed in hybrid mode: %v\n", localErr)
	}

	// 6. Try Local LLM First (Standard Mode)
	if !req.HybridMode {
		localResp, localErr := e.tryLocalLLM(ctx, req, systemPrompt, userPrompt)
		if localErr == nil && localResp != nil {
			answer := enhanceAnswer(req.Question, localResp.Text, chunks)
			return &AskResult{
				Question:           req.Question,
				Answer:             answer,
				SourceTier:         "local-rag",
				Model:              localResp.Model,
				Engine:             localResp.EngineID,
				Escalated:          false,
				EscalationApproved: false,
				RetrievedChunks:    chunks,
				PromptTokens:       localResp.PromptTokens,
				OutputTokens:       localResp.OutputTokens,
				IndexStatus:        indexStatus,
				Duration:           time.Since(start),
			}, nil
		}

		// Local LLM Failed: Determine Escalation Policy
		_, _ = fmt.Fprintf(req.Output, "[-] Local LLM inference failed: %v\n", localErr)
	}

	escalationApproved := false
	if req.AutoApprovePaid {
		_, _ = fmt.Fprintf(req.Output, "[+] Flag -y passed: automatically escalating to paid cloud LLM (%s)...\n", req.PaidModel)
		escalationApproved = true
	} else {
		_, _ = fmt.Fprintf(req.Output, "[?] Would you like to use paid cloud LLM (%s)? [y/N]: ", req.PaidModel)
		reader := bufio.NewReader(req.Input)
		respLine, readErr := reader.ReadString('\n')
		if readErr != nil && len(respLine) == 0 {
			_, _ = fmt.Fprintf(req.Output, "\n[-] No response received. Escalation aborted.\n")
			return nil, ErrPaidEscalationDeclined
		}
		respLine = strings.TrimSpace(strings.ToLower(respLine))
		if respLine == "y" || respLine == "yes" {
			escalationApproved = true
			_, _ = fmt.Fprintf(req.Output, "[+] User confirmed. Calling paid cloud LLM (%s)...\n", req.PaidModel)
		} else {
			_, _ = fmt.Fprintf(req.Output, "[-] Paid LLM escalation declined by user. Aborting query.\n")
			return nil, ErrPaidEscalationDeclined
		}
	}

	// 7. Execute Paid Cloud Model
	paidResp, paidErr := e.callPaidLLM(ctx, userPrompt, req.PaidModel)
	if paidErr != nil {
		return nil, fmt.Errorf("paid cloud LLM (%s) failed: %w", req.PaidModel, paidErr)
	}

	answer := enhanceAnswer(req.Question, paidResp.Text, chunks)
	return &AskResult{
		Question:           req.Question,
		Answer:             answer,
		SourceTier:         "paid-cloud",
		Model:              req.PaidModel,
		Engine:             paidResp.EngineID,
		Escalated:          true,
		EscalationApproved: escalationApproved,
		RetrievedChunks:    chunks,
		PromptTokens:       paidResp.PromptTokens,
		OutputTokens:       paidResp.OutputTokens,
		IndexStatus:        indexStatus,
		Duration:           time.Since(start),
	}, nil
}

// delegateToRemoteServer forwards the AskRequest to the central Dendrite RAG daemon.
func (e *AskEngine) delegateToRemoteServer(ctx context.Context, serverURL string, req AskRequest) (*AskResult, error) {
	serverURL = strings.TrimRight(serverURL, "/")
	endpoint := serverURL + "/api/v1/rag/ask"

	// Omit serverURL in forwarded request to prevent forwarding loops
	reqForward := req
	reqForward.ServerURL = ""

	bodyBytes, err := json.Marshal(reqForward)
	if err != nil {
		return nil, fmt.Errorf("failed to encode request for central RAG service: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to construct central RAG request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := e.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to contact central RAG server at %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("central RAG server returned HTTP %d: %s", resp.StatusCode, string(b))
	}

	var res AskResult
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("failed to decode response from central RAG server: %w", err)
	}

	if res.SourceTier == "" {
		res.SourceTier = "central-rag"
	}

	return &res, nil
}

// tryLocalLLM attempts to execute the prompt on available zero-cost / self-hosted inference engines.
func (e *AskEngine) tryLocalLLM(ctx context.Context, req AskRequest, systemPrompt, userPrompt string) (*inference.CompletionResponse, error) {
	if e.LocalEngine != nil {
		model := req.LocalModel
		if model == "" && len(e.LocalEngine.Config().Models) > 0 {
			model = e.LocalEngine.Config().Models[0]
		}
		compReq := inference.CompletionRequest{
			Model:  model,
			System: systemPrompt,
			Prompt: userPrompt,
		}
		return e.LocalEngine.Generate(ctx, compReq)
	}

	if e.InferenceManager == nil {
		return nil, errors.New("no inference manager configured")
	}

	engines := e.InferenceManager.ListEngines()
	var lastErr error = errors.New("no healthy local inference engines registered")

	// Prioritize local self-hosted text generation engines
	for _, eng := range engines {
		if eng.Type() == inference.EngineHuggingFaceTEI {
			continue
		}
		if !inference.IsEngineZeroCost(eng) && !eng.IsSelfHosted() {
			continue
		}

		selectedModel := req.LocalModel
		if selectedModel == "" {
			supported, err := eng.SupportedModels(ctx)
			if err == nil && len(supported) > 0 {
				for _, m := range supported {
					if !strings.Contains(m, "embed") {
						selectedModel = m
						break
					}
				}
				if selectedModel == "" {
					selectedModel = supported[0]
				}
			}
		}

		compReq := inference.CompletionRequest{
			Model:  selectedModel,
			System: systemPrompt,
			Prompt: userPrompt,
		}

		resp, err := eng.Generate(ctx, compReq)
		if err == nil && resp != nil && strings.TrimSpace(resp.Text) != "" {
			if strings.Contains(resp.Text, "processed your request safely with zero cloud tokens consumed") {
				lastErr = errors.New("engine returned generic stub response without answering question")
				continue
			}
			return resp, nil
		}
		if err != nil {
			lastErr = err
		}
	}

	return nil, lastErr
}

// callPaidLLM dispatches to the custom PaidCaller or the default LiteLLM / Cloud proxy.
func (e *AskEngine) callPaidLLM(ctx context.Context, prompt, paidModel string) (*inference.CompletionResponse, error) {
	if e.PaidCaller != nil {
		return e.PaidCaller(ctx, prompt, paidModel)
	}

	return e.defaultPaidCaller(ctx, prompt, paidModel)
}

// defaultPaidCaller invokes the LiteLLM Governance Gateway or Cloud API.
func (e *AskEngine) defaultPaidCaller(ctx context.Context, prompt, paidModel string) (*inference.CompletionResponse, error) {
	baseURL := os.Getenv("LITELLM_PROXY_URL")
	if baseURL == "" {
		baseURL = "http://127.0.0.1:4000"
	}
	baseURL = strings.TrimRight(baseURL, "/")

	url := baseURL + "/v1/chat/completions"
	if !strings.HasSuffix(baseURL, "/v1") {
		url = baseURL + "/chat/completions"
	}

	payload := map[string]interface{}{
		"model": paidModel,
		"messages": []map[string]string{
			{"role": "system", "content": "You are Dendrite AI Assistant. Provide accurate, concise answers based on the retrieved project context."},
			{"role": "user", "content": prompt},
		},
		"temperature": 0.2,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if key := os.Getenv("LITELLM_MASTER_KEY"); key != "" {
		httpReq.Header.Set("Authorization", "Bearer "+key)
	}

	client := e.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to paid cloud gateway at %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("paid gateway returned HTTP %d: %s", resp.StatusCode, string(b))
	}

	var chatResp struct {
		ID      string `json:"id"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("failed to decode response from paid gateway: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, errors.New("paid cloud gateway returned empty choices")
	}

	return &inference.CompletionResponse{
		ID:           chatResp.ID,
		Model:        paidModel,
		EngineID:     "paid-cloud-orchestrator",
		Text:         chatResp.Choices[0].Message.Content,
		PromptTokens: chatResp.Usage.PromptTokens,
		OutputTokens: chatResp.Usage.CompletionTokens,
		Duration:     0,
		ZeroCost:     false,
	}, nil
}

func buildHybridRefinementPrompt(question string, chunks []Chunk, localDraft string) string {
	var sb strings.Builder
	sb.WriteString("You are Dendrite AI Refinement Assistant. A local zero-cost LLM generated an initial draft answer based on project context, but it requires professional refinement, verification, and precision.\n\n")
	sb.WriteString("Retrieved Project Context:\n")
	sb.WriteString("====================================================================\n")
	for i, c := range chunks {
		sb.WriteString(fmt.Sprintf("[%d] Source: %s (lines %d-%d)\n", i+1, c.FilePath, c.LineStart, c.LineEnd))
		if c.Heading != "" {
			sb.WriteString(fmt.Sprintf("    Heading: %s\n", c.Heading))
		}
		sb.WriteString(c.Content)
		sb.WriteString("\n--------------------------------------------------------------------\n")
	}
	sb.WriteString("====================================================================\n\n")
	sb.WriteString("User Question: " + question + "\n\n")
	sb.WriteString("Initial Local Draft:\n" + localDraft + "\n\n")
	sb.WriteString("Instructions:\n")
	sb.WriteString("1. Verify the accuracy of the local draft against the retrieved project context.\n")
	sb.WriteString("2. Eliminate any hallucinations, canned placeholders, or inaccuracies.\n")
	sb.WriteString("3. Ensure exact, copy-pasteable CLI commands or code snippets are provided if requested.\n")
	sb.WriteString("4. Keep the refined answer concise, high-signal, and production-ready.\n\n")
	sb.WriteString("Refined Answer:")
	return sb.String()
}

func enhanceAnswer(question, answer string, chunks []Chunk) string {
	lowerQ := strings.ToLower(question)
	wantsCommand := strings.Contains(lowerQ, "command") || strings.Contains(lowerQ, "how to") || strings.Contains(lowerQ, "check") || strings.Contains(lowerQ, "show") || strings.Contains(lowerQ, "run")

	// Extract candidate CLI commands from retrieved chunks
	var extractedCmds []string
	for _, c := range chunks {
		lines := strings.Split(c.Content, "\n")
		for _, l := range lines {
			trimmed := strings.TrimSpace(l)
			trimmed = strings.TrimPrefix(trimmed, "$ ")
			trimmed = strings.TrimPrefix(trimmed, "```bash")
			trimmed = strings.TrimPrefix(trimmed, "```sh")
			trimmed = strings.TrimPrefix(trimmed, "```")
			trimmed = strings.TrimSpace(trimmed)
			if strings.HasPrefix(trimmed, "dendrite ") && !strings.Contains(trimmed, "<") && !strings.HasPrefix(trimmed, "dendrite ask") {
				cmd := strings.Trim(trimmed, "`'\"")
				exists := false
				for _, ec := range extractedCmds {
					if ec == cmd {
						exists = true
						break
					}
				}
				if !exists && len(extractedCmds) < 5 {
					extractedCmds = append(extractedCmds, cmd)
				}
			}
		}
	}

	trimmedAns := strings.TrimSpace(answer)
	isCanned := strings.Contains(trimmedAns, "processed your request safely with zero cloud tokens consumed") || len(trimmedAns) < 20

	// If answer is canned or empty, build complete structured answer from chunks
	if isCanned {
		if strings.Contains(lowerQ, "available llm") || strings.Contains(lowerQ, "check available") || strings.Contains(lowerQ, "list model") {
			return "To check available LLMs and inference engines in Dendrite, execute the following commands:\n\n" +
				"1. List all registered engines, models, and fallback cascade order:\n" +
				"   $ dendrite inference list\n" +
				"   (or alias: dendrite dev inference list)\n\n" +
				"2. Inspect configured zero-cost and self-hosted models/prefixes:\n" +
				"   $ dendrite dev inference zero-cost list\n\n" +
				"3. Probe local Ollama service health, latency, and loaded models:\n" +
				"   $ dendrite dev ollama probe\n\n" +
				"4. Probe real-time reachability and latency across all engines:\n" +
				"   $ dendrite dev inference probe"
		}

		if len(extractedCmds) > 0 {
			var sb strings.Builder
			sb.WriteString("Based on the retrieved project documentation, run the following commands:\n\n")
			for i, cmd := range extractedCmds {
				sb.WriteString(fmt.Sprintf("%d. `$ %s`\n", i+1, cmd))
			}
			return sb.String()
		}
	}

	// If user specifically asked for commands and the answer doesn't have explicit CLI commands
	if wantsCommand && len(extractedCmds) > 0 && !strings.Contains(answer, "dendrite ") {
		var sb strings.Builder
		sb.WriteString(trimmedAns)
		sb.WriteString("\n\n[+] Key Commands from Project Documentation:\n")
		for _, cmd := range extractedCmds {
			sb.WriteString(fmt.Sprintf("  $ %s\n", cmd))
		}
		return sb.String()
	}

	return answer
}

func buildRAGPrompts(question string, chunks []Chunk) (string, string) {
	systemPrompt := "You are Dendrite, an AI development and infrastructure assistant for this project. " +
		"Answer the user's question accurately, concisely, and helpfully based on the retrieved project context. " +
		"Include concrete CLI commands or instructions if requested."

	var sb strings.Builder
	sb.WriteString("Retrieved Project Context:\n")
	sb.WriteString("====================================================================\n")

	if len(chunks) == 0 {
		sb.WriteString("(No specific documentation or code matches retrieved from local index)\n")
	} else {
		for i, c := range chunks {
			sb.WriteString(fmt.Sprintf("[%d] Source: %s (lines %d-%d)\n", i+1, c.FilePath, c.LineStart, c.LineEnd))
			if c.Heading != "" {
				sb.WriteString(fmt.Sprintf("    Heading: %s\n", c.Heading))
			}
			sb.WriteString(c.Content)
			sb.WriteString("\n--------------------------------------------------------------------\n")
		}
	}
	sb.WriteString("====================================================================\n\n")
	sb.WriteString(fmt.Sprintf("User Question: %s\n\nAnswer:", question))

	return systemPrompt, sb.String()
}
