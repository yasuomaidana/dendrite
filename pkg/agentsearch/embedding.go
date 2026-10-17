package agentsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"
)

var (
	// ErrEmptyText indicates an embedding request was made with no text.
	ErrEmptyText = errors.New("empty text input for embedding")
	// ErrDimensionMismatch indicates vectors have different dimensions during similarity calculation.
	ErrDimensionMismatch = errors.New("vector dimension mismatch")
)

// EmbeddingClient is the Strategy interface for dense vector embedding generators.
type EmbeddingClient interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Dimension() int
}

// TEIEmbeddingClient connects to a Hugging Face Text Embeddings Inference (TEI) instance.
type TEIEmbeddingClient struct {
	endpoint   string
	httpClient *http.Client
	dimension  int
}

// NewTEIEmbeddingClient creates an adapter for Hugging Face TEI.
func NewTEIEmbeddingClient(endpoint string, client *http.Client) *TEIEmbeddingClient {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	if endpoint == "" {
		endpoint = "http://localhost:8082"
	}
	return &TEIEmbeddingClient{
		endpoint:   strings.TrimRight(endpoint, "/"),
		httpClient: client,
		dimension:  384, // Default for bge-small-en-v1.5
	}
}

// Dimension returns the vector dimension.
func (c *TEIEmbeddingClient) Dimension() int {
	return c.dimension
}

// Embed generates dense vectors via the TEI /embed endpoint.
func (c *TEIEmbeddingClient) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, ErrEmptyText
	}

	payload := map[string]interface{}{
		"inputs": texts,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/embed", c.endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tei request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tei returned HTTP %d", resp.StatusCode)
	}

	var embeddings [][]float32
	if err := json.NewDecoder(resp.Body).Decode(&embeddings); err != nil {
		return nil, fmt.Errorf("failed to decode tei embeddings: %w", err)
	}

	if len(embeddings) > 0 && len(embeddings[0]) > 0 {
		c.dimension = len(embeddings[0])
	}
	return embeddings, nil
}

// OllamaEmbeddingClient connects to a local Ollama instance for embeddings.
type OllamaEmbeddingClient struct {
	endpoint   string
	model      string
	httpClient *http.Client
	dimension  int
}

// NewOllamaEmbeddingClient creates an adapter for Ollama embeddings.
func NewOllamaEmbeddingClient(endpoint, model string, client *http.Client) *OllamaEmbeddingClient {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	if endpoint == "" {
		endpoint = "http://localhost:11434"
	}
	if model == "" {
		model = "nomic-embed-text"
	}
	return &OllamaEmbeddingClient{
		endpoint:   strings.TrimRight(endpoint, "/"),
		model:      model,
		httpClient: client,
		dimension:  768,
	}
}

// Dimension returns the vector dimension.
func (c *OllamaEmbeddingClient) Dimension() int {
	return c.dimension
}

// Embed generates embeddings using Ollama's /api/embeddings endpoint.
func (c *OllamaEmbeddingClient) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, ErrEmptyText
	}

	results := make([][]float32, len(texts))
	for i, text := range texts {
		payload := map[string]interface{}{
			"model":  c.model,
			"prompt": text,
		}
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}

		url := fmt.Sprintf("%s/api/embeddings", c.endpoint)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("ollama embedding failed: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("ollama returned HTTP %d", resp.StatusCode)
		}

		var res struct {
			Embedding []float32 `json:"embedding"`
		}
		err = json.NewDecoder(resp.Body).Decode(&res)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to decode ollama embedding: %w", err)
		}

		results[i] = res.Embedding
		if len(res.Embedding) > 0 {
			c.dimension = len(res.Embedding)
		}
	}

	return results, nil
}

// CosineSimilarity computes the cosine similarity between two normalized vectors.
func CosineSimilarity(a, b []float32) (float32, error) {
	if len(a) != len(b) {
		return 0, ErrDimensionMismatch
	}
	if len(a) == 0 {
		return 0, nil
	}

	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i] * b[i])
		normA += float64(a[i] * a[i])
		normB += float64(b[i] * b[i])
	}

	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0, nil
	}

	sim := float32(dot / denom)
	if sim < 0 {
		return 0, nil
	}
	if sim > 1.0 {
		return 1.0, nil
	}
	return sim, nil
}
