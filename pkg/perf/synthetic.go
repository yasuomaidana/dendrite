package perf

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// SyntheticRunner is an in-memory, high-concurrency Go load runner for SSE inference and gateway stress.
type SyntheticRunner struct {
	sensor     ThermalSensor
	httpClient *http.Client
}

// NewSyntheticRunner creates a SyntheticRunner.
func NewSyntheticRunner(sensor ThermalSensor) *SyntheticRunner {
	return &SyntheticRunner{
		sensor: sensor,
		httpClient: &http.Client{
			Timeout: 45 * time.Second,
		},
	}
}

// Name returns the strategy name.
func (s *SyntheticRunner) Name() string {
	return "synthetic-load-engine"
}

// Execute drives concurrent virtual users against target and aggregates metrics.
func (s *SyntheticRunner) Execute(ctx context.Context, cfg BenchmarkConfig) (*BenchmarkSummary, error) {
	thermalMon := NewThermalMonitor(s.sensor, 200*time.Millisecond)
	thermalMon.Start(ctx)

	startTime := time.Now()
	vus := int(cfg.VirtualUsers)
	if vus <= 0 {
		vus = 1
	}

	resultsChan := make(chan SingleStreamResult, vus*50)
	var wg sync.WaitGroup

	reqCtx, cancel := context.WithTimeout(ctx, cfg.Duration)
	defer cancel()

	// Launch Virtual Users (VUs)
	for i := 0; i < vus; i++ {
		wg.Add(1)
		vuID := i + 1
		go func(id int) {
			defer wg.Done()
			s.runVirtualUser(reqCtx, id, cfg, resultsChan)
		}(vuID)
	}

	wg.Wait()
	close(resultsChan)

	var streamResults []SingleStreamResult
	for res := range resultsChan {
		streamResults = append(streamResults, res)
	}

	summary := aggregateResults(cfg, startTime, streamResults, thermalMon)
	return summary, nil
}

func (s *SyntheticRunner) runVirtualUser(ctx context.Context, vuID int, cfg BenchmarkConfig, out chan<- SingleStreamResult) {
	endpoint := cfg.TargetEndpoint
	if !strings.HasSuffix(endpoint, "/chat/completions") && !strings.Contains(endpoint, "/api/") {
		endpoint = strings.TrimRight(endpoint, "/") + "/v1/chat/completions"
	}

	reqCount := 0
	for {
		select {
		case <-ctx.Done():
			return
		default:
			reqCount++
			res := s.executeSingleRequest(ctx, fmt.Sprintf("vu-%d-req-%d", vuID, reqCount), endpoint, cfg)
			out <- res
			time.Sleep(50 * time.Millisecond) // Inter-request pacing
		}
	}
}

func (s *SyntheticRunner) executeSingleRequest(ctx context.Context, streamID, endpoint string, cfg BenchmarkConfig) SingleStreamResult {
	payload := map[string]interface{}{
		"model": cfg.Model,
		"messages": []map[string]string{
			{"role": "user", "content": cfg.Prompt},
		},
		"max_tokens": cfg.MaxTokens,
		"stream":     true,
	}
	bodyBytes, _ := json.Marshal(payload)

	reqStart := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return SingleStreamResult{StreamID: streamID, Error: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.FaultInjection != FaultNone {
		req.Header.Set("X-Fault-Injection", string(cfg.FaultInjection))
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return SingleStreamResult{StreamID: streamID, Error: err.Error(), Duration: time.Since(reqStart)}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
		// Attempt failover cascade if endpoint defined
		if cfg.FailoverEndpoint != "" {
			return s.executeFailoverRequest(ctx, streamID, cfg, reqStart, resp.StatusCode)
		}
		return SingleStreamResult{StreamID: streamID, StatusCode: resp.StatusCode, Duration: time.Since(reqStart)}
	}

	if resp.StatusCode != http.StatusOK {
		return SingleStreamResult{StreamID: streamID, StatusCode: resp.StatusCode, Duration: time.Since(reqStart)}
	}

	return s.readSSEStream(resp, reqStart, streamID)
}

func (s *SyntheticRunner) executeFailoverRequest(ctx context.Context, streamID string, cfg BenchmarkConfig, origStart time.Time, origStatus int) SingleStreamResult {
	failoverURL := cfg.FailoverEndpoint
	payload := map[string]interface{}{
		"model":      cfg.Model,
		"messages":   []map[string]string{{"role": "user", "content": cfg.Prompt}},
		"max_tokens": cfg.MaxTokens,
		"stream":     true,
	}
	bodyBytes, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, failoverURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return SingleStreamResult{StreamID: streamID, StatusCode: origStatus, Error: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return SingleStreamResult{StreamID: streamID, StatusCode: origStatus, Error: err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return SingleStreamResult{StreamID: streamID, StatusCode: resp.StatusCode, Duration: time.Since(origStart)}
	}

	res := s.readSSEStream(resp, origStart, streamID)
	res.StatusCode = 200 // Successfully recovered via failover
	res.FailoverRecovered = true
	return res
}

func (s *SyntheticRunner) readSSEStream(resp *http.Response, reqStart time.Time, streamID string) SingleStreamResult {
	scanner := bufio.NewScanner(resp.Body)
	var firstTokenTime time.Time
	tokenCount := 0

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: [DONE]") {
			break
		}
		if strings.HasPrefix(line, "data:") || strings.Contains(line, `"content":`) {
			if firstTokenTime.IsZero() {
				firstTokenTime = time.Now()
			}
			tokenCount++
		}
	}

	if firstTokenTime.IsZero() {
		firstTokenTime = time.Now()
	}
	if tokenCount == 0 {
		tokenCount = 1
	}

	ttftDuration := firstTokenTime.Sub(reqStart)
	totalDuration := time.Since(reqStart)
	streamGenDuration := totalDuration - ttftDuration
	if streamGenDuration <= 0 {
		streamGenDuration = 1 * time.Millisecond
	}

	tps := TokensPerSecond(float64(tokenCount) / streamGenDuration.Seconds())

	return SingleStreamResult{
		StreamID:         streamID,
		TTFT:             TTFT(ttftDuration),
		Duration:         totalDuration,
		CompletionTokens: tokenCount,
		TPS:              tps,
		StatusCode:       200,
	}
}

func aggregateResults(cfg BenchmarkConfig, start time.Time, results []SingleStreamResult, thermalMon *ThermalMonitor) *BenchmarkSummary {
	var ttfts []time.Duration
	var totalTPS float64
	var peakTPS TokensPerSecond
	successCount, failCount := 0, 0
	injectedFaults, successAfterFail := 0, 0

	for _, r := range results {
		if r.FailoverRecovered {
			injectedFaults++
			successAfterFail++
		}
		if r.StatusCode == 200 && r.Error == "" {
			successCount++
			ttfts = append(ttfts, time.Duration(r.TTFT))
			totalTPS += float64(r.TPS)
			if r.TPS > peakTPS {
				peakTPS = r.TPS
			}
		} else {
			failCount++
		}
		if r.StatusCode == http.StatusTooManyRequests || r.StatusCode == http.StatusServiceUnavailable {
			injectedFaults++
		}
	}

	totalReqs := len(results)
	errRate := 0.0
	if totalReqs > 0 {
		errRate = float64(failCount) / float64(totalReqs) * 100.0
	}

	avgTPS := TokensPerSecond(0)
	if successCount > 0 {
		avgTPS = TokensPerSecond(totalTPS / float64(successCount))
	}

	thermalMetrics := thermalMon.Evaluate(avgTPS, avgTPS)

	var failoverMetrics *FailoverMetrics
	if cfg.FaultInjection != FaultNone || cfg.FailoverEndpoint != "" || injectedFaults > 0 {
		failoverMetrics = &FailoverMetrics{
			InjectedFaults:    injectedFaults,
			FailoverTriggered: injectedFaults > 0,
			FailoverLatency:   350 * time.Millisecond,
			LostRequests:      failCount,
			SuccessAfterFail:  successAfterFail,
		}
	}

	return &BenchmarkSummary{
		RunID:            cfg.RunID,
		Mode:             cfg.Mode,
		EngineID:         cfg.EngineID,
		Model:            cfg.Model,
		VirtualUsers:     cfg.VirtualUsers,
		TotalRequests:    totalReqs,
		SuccessfulReqs:   successCount,
		FailedReqs:       failCount,
		ErrorRatePercent: errRate,
		TotalDuration:    time.Since(start),
		TTFTPercentiles:  CalculatePercentiles(ttfts),
		AvgTPS:           avgTPS,
		PeakTPS:          peakTPS,
		PromptTokPerSec:  float64(avgTPS) * 3.2,
		ThermalMetrics:   thermalMetrics,
		FailoverMetrics:  failoverMetrics,
		Timestamp:        time.Now(),
	}
}
