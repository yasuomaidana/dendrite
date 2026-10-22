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
	"path/filepath"
	"strings"
	"time"

	"Dendrite/pkg/sidecar"
)

// DefaultPaidModel is the paid model when a request names none.
const DefaultPaidModel = "gemini-2.5-pro"

// AskContextTokens is the context-pack budget for a question (the sidecar's `ask` uses the
// same).
const AskContextTokens = 2500

var (
	// ErrEmptyQuestion is returned when the user question is blank.
	ErrEmptyQuestion = errors.New("question cannot be empty")
	// ErrPaidEscalationDeclined indicates the human refused paid LLM escalation.
	ErrPaidEscalationDeclined = errors.New("local LLM failed and paid cloud escalation declined by user")
)

// AskEngine answers questions about a project through the dendrite-rag sidecar's `ask`
// method: retrieval, prompts and the local, hybrid and paid tiers run in the sidecar, which
// calls models through the LiteLLM gateway. Dendrite decides paid escalation with the
// person and can forward the whole question to a central Dendrite server instead.
type AskEngine struct {
	Retriever  *Retriever // the dendrite-rag sidecar
	HTTPClient *http.Client
	// PickLocal chooses the local model when the request names none (default:
	// PreferLocalModel, GPU engines first).
	PickLocal func(ctx context.Context, gatewayKey string) (LocalChoice, bool)
}

// NewAskEngine returns an AskEngine on the default sidecar locations.
func NewAskEngine() *AskEngine {
	return &AskEngine{
		Retriever:  NewRetriever(),
		HTTPClient: &http.Client{Timeout: 60 * time.Second},
		PickLocal:  PreferLocalModel,
	}
}

// sidecarAsk is the `ask` method's params (docs/specs/ask.md in dendrite-rag-rs).
type sidecarAsk struct {
	Root       string `json:"root"`
	Question   string `json:"question"`
	Mode       string `json:"mode"`
	TopK       int    `json:"top_k,omitempty"`
	Corpus     string `json:"corpus,omitempty"`
	LocalModel string `json:"local_model,omitempty"`
	PaidModel  string `json:"paid_model,omitempty"`
	AllowPaid  bool   `json:"allow_paid"`
	Fresh      bool   `json:"fresh,omitempty"`
	// Paths restricts retrieval to a single-file project's file.
	Paths []string `json:"paths,omitempty"`
}

// sidecarAnswer is the `ask` method's result.
type sidecarAnswer struct {
	Answer           string       `json:"answer"`
	SourceTier       string       `json:"source_tier"`
	Model            string       `json:"model"`
	LocalDraft       string       `json:"local_draft"`
	EscalationNeeded bool         `json:"escalation_needed"`
	LocalError       string       `json:"local_error"`
	Escalated        bool         `json:"escalated"`
	PromptTokens     int          `json:"prompt_tokens"`
	OutputTokens     int          `json:"output_tokens"`
	Hits             []sidecarHit `json:"hits"`
	Cached           bool         `json:"cached"`
	Timings          struct {
		Retrieval struct {
			Total int64 `json:"total_ms"`
		} `json:"retrieval"`
		Local int64 `json:"local_ms"`
		Paid  int64 `json:"paid_ms"`
	} `json:"timings"`
}

// Ask answers req.Question from the project's index. When the local model fails and paid
// escalation was not approved up front (-y), the person is asked before paying.
func (e *AskEngine) Ask(ctx context.Context, req AskRequest) (*AskResult, error) {
	start := time.Now()
	req.Question = strings.TrimSpace(req.Question)
	if req.Question == "" {
		return nil, ErrEmptyQuestion
	}
	serverURL := req.ServerURL
	if serverURL == "" {
		serverURL = os.Getenv("DENDRITE_RAG_SERVER")
	}
	if serverURL != "" {
		return e.delegateToRemoteServer(ctx, serverURL, req)
	}
	if req.ProjectPath == "" {
		req.ProjectPath = "."
	}
	if req.PaidModel == "" {
		req.PaidModel = DefaultPaidModel
	}
	if req.Corpus == "" {
		req.Corpus = "all"
	}
	if req.Input == nil {
		req.Input = os.Stdin
	}
	if req.Output == nil {
		req.Output = os.Stdout
	}
	mode := "local"
	switch {
	case req.ForcePaid:
		mode = "paid"
		_, _ = fmt.Fprintf(req.Output, "[+] Force-paid flag enabled: querying paid cloud LLM (%s) directly with project RAG context...\n", req.PaidModel)
	case req.HybridMode:
		mode = "hybrid"
	}
	retriever := NewRetriever()
	if e.Retriever != nil {
		copied := *e.Retriever
		retriever = &copied
	}
	gateway := gatewayEnv(req.ProjectPath, retriever.Locator.MemberDirs)
	retriever.Env = append(retriever.Env, gateway...)
	engine := "dendrite-rag"
	// chooseLocal picks a GPU-served local model unless the request or the project's
	// [ask] local_model (settings file or DENDRITE_RAG_ASK_LOCAL_MODEL) names one.
	chooseLocal := func(c *sidecar.Client, root string) {
		if req.LocalModel != "" || mode == "paid" || e.PickLocal == nil {
			return
		}
		var st struct {
			Settings struct {
				Ask struct {
					LocalModel string `json:"local_model"`
				} `json:"ask"`
			} `json:"settings"`
		}
		if err := c.Call(ctx, "settings", map[string]interface{}{"root": root}, &st); err == nil && st.Settings.Ask.LocalModel != "" {
			return
		}
		key := os.Getenv("LITELLM_MASTER_KEY")
		if len(gateway) > 0 {
			key = strings.TrimPrefix(gateway[0], "LITELLM_MASTER_KEY=")
		}
		if choice, ok := e.PickLocal(ctx, key); ok {
			req.LocalModel, engine = choice.Model, string(choice.Engine)
			_, _ = fmt.Fprintf(req.Output, "[i] Local model: %s\n", choice)
		}
	}

	var answer sidecarAnswer
	// --paid and -y are consent given up front.
	approved := req.AutoApprovePaid || req.ForcePaid
	retrieval, err := retriever.run(ctx, req.ProjectPath, req.UpdateIndex, req.Question, func(c *sidecar.Client, root string, only []string, out *Retrieval) error {
		if minor := protocolMinor(c.Health.Protocol); minor < AskProtocolMinor {
			return fmt.Errorf("%w: %s speaks protocol %s; dendrite ask needs 1.%d or later; %s",
				ErrSidecarTooOld, c.Path, c.Health.Protocol, AskProtocolMinor, InstallHint)
		}
		chooseLocal(c, root)
		params := sidecarAsk{
			Root: root, Question: req.Question, Mode: mode, TopK: req.TopK, Corpus: req.Corpus,
			LocalModel: req.LocalModel, PaidModel: req.PaidModel, AllowPaid: approved,
			// A re-indexed project also wants a fresh answer, not the cached one.
			Fresh: req.UpdateIndex,
			Paths: only,
		}
		if err := c.Call(ctx, "ask", params, &answer); err != nil {
			return fmt.Errorf("ask: %w", err)
		}
		if !answer.EscalationNeeded {
			return nil
		}
		if !confirmPaid(req, answer.LocalError) {
			return ErrPaidEscalationDeclined
		}
		approved = true
		// The local model already failed: go straight to the paid one instead of
		// letting the sidecar try the local model again.
		params.Mode, params.AllowPaid = "paid", true
		answer = sidecarAnswer{}
		if err := c.Call(ctx, "ask", params, &answer); err != nil {
			return fmt.Errorf("ask: %w", err)
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrPaidEscalationDeclined) {
			return nil, ErrPaidEscalationDeclined
		}
		return nil, fmt.Errorf("asking the project index: %w", err)
	}
	switch {
	case answer.SourceTier == "hybrid":
		_, _ = fmt.Fprintf(req.Output, "[+] Hybrid mode: local draft refined by the paid model (%s).\n", req.PaidModel)
	case req.HybridMode && answer.LocalError != "" && answer.SourceTier == "local-rag":
		_, _ = fmt.Fprintf(req.Output, "[-] Paid refinement failed (%s), returning verified local draft.\n", answer.LocalError)
	}
	if retrieval.EmbedError != "" {
		_, _ = fmt.Fprintf(req.Output, "[!] Embeddings unavailable (%s); answering from lexical and symbol matches\n", retrieval.EmbedError)
	}
	if retrieval.EmbedPending > 0 {
		_, _ = fmt.Fprintf(req.Output, "[!] Dense retrieval covers %.0f%% of the project (%d chunks still without embeddings); finish them once with: dendrite-rag index %s\n",
			retrieval.EmbedCoverage*100, retrieval.EmbedPending, req.ProjectPath)
	}
	return &AskResult{
		Question:           req.Question,
		Answer:             answer.Answer,
		LocalDraft:         answer.LocalDraft,
		SourceTier:         answer.SourceTier,
		Model:              answer.Model,
		Engine:             engine,
		Escalated:          answer.Escalated,
		EscalationApproved: answer.Escalated && approved,
		RetrievedChunks:    toChunks(req.ProjectPath, singleFile(req.ProjectPath), answer.Hits),
		PromptTokens:       answer.PromptTokens,
		OutputTokens:       answer.OutputTokens,
		IndexStatus:        retrieval.IndexStatus,
		Cached:             answer.Cached,
		Duration:           time.Since(start),
		Timings: &AskTimings{
			Spawn:     retrieval.SpawnMS,
			Index:     retrieval.IndexMS,
			Embed:     retrieval.EmbedMS,
			Retrieval: answer.Timings.Retrieval.Total,
			Local:     answer.Timings.Local,
			Paid:      answer.Timings.Paid,
		},
	}, nil
}

// gatewayEnv passes the LiteLLM gateway key to the sidecar when the environment does not
// export it: it is read from the project's .env, then from the .env of the synapse member
// checkouts (the Dendrite control plane keeps it), as detached agent jobs do (the sidecar
// calls the gateway for `ask`).
func gatewayEnv(projectPath string, memberDirs []string) []string {
	if os.Getenv("LITELLM_MASTER_KEY") != "" {
		return nil
	}
	var dirs []string
	if root, _, err := projectRoot(projectPath); err == nil {
		dirs = append(dirs, root)
	}
	for _, dir := range append(dirs, memberDirs...) {
		data, err := os.ReadFile(filepath.Join(dir, ".env"))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			if v, ok := strings.CutPrefix(strings.TrimSpace(line), "LITELLM_MASTER_KEY="); ok {
				if v = strings.Trim(strings.TrimSpace(v), `"'`); v != "" {
					return []string{"LITELLM_MASTER_KEY=" + v}
				}
			}
		}
	}
	return nil
}

// confirmPaid asks the person whether to pay for the cloud model after the local one
// failed; no answer is a no.
func confirmPaid(req AskRequest, localError string) bool {
	if localError != "" {
		_, _ = fmt.Fprintf(req.Output, "[-] Local LLM inference failed: %s\n", localError)
	}
	_, _ = fmt.Fprintf(req.Output, "[?] Would you like to use paid cloud LLM (%s)? [y/N]: ", req.PaidModel)
	line, err := bufio.NewReader(req.Input).ReadString('\n')
	if err != nil && line == "" {
		_, _ = fmt.Fprintf(req.Output, "\n[-] No response received. Escalation aborted.\n")
		return false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		_, _ = fmt.Fprintf(req.Output, "[+] User confirmed. Calling paid cloud LLM (%s)...\n", req.PaidModel)
		return true
	}
	_, _ = fmt.Fprintf(req.Output, "[-] Paid LLM escalation declined by user. Aborting query.\n")
	return false
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
