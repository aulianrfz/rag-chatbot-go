// internal/rag/embedder.go
// Embedding provider interface + dua implementasi:
//   - GeminiEmbedder  → pakai Gemini API (butuh API key)
//   - OllamaEmbedder  → pakai Ollama lokal (tidak butuh API key)
//
// Provider dipilih otomatis dari config:
//   LLM_PROVIDER=ollama  → OllamaEmbedder (model: nomic-embed-text)
//   LLM_PROVIDER=gemini  → GeminiEmbedder (model: text-embedding-004)

package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ── Interface ───────────────────────────────────────────────

// Embedder mengubah teks menjadi vektor float32.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
	Dimensions() int
}

// ── Shared HTTP client ───────────────────────────────────────

func newHTTPClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}

// embedBatchSequential adalah helper untuk loop batch secara sequential.
// Dipakai oleh kedua implementasi karena keduanya tidak punya true batch endpoint.
func embedBatchSequential(ctx context.Context, texts []string, embedFn func(context.Context, string) ([]float32, error)) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i, text := range texts {
		vec, err := embedFn(ctx, text)
		if err != nil {
			return nil, fmt.Errorf("embed teks ke-%d: %w", i, err)
		}
		results[i] = vec

		if i < len(texts)-1 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(50 * time.Millisecond):
			}
		}
	}
	return results, nil
}

// ── Gemini Embedder ─────────────────────────────────────────

const (
	geminiEmbedURL  = "https://generativelanguage.googleapis.com/v1beta/models/%s:embedContent?key=%s"
	geminiEmbedDims = 768
)

// GeminiEmbedder memanggil Gemini Embedding API.
type GeminiEmbedder struct {
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewGeminiEmbedder membuat GeminiEmbedder baru.
func NewGeminiEmbedder(apiKey, model string) *GeminiEmbedder {
	return &GeminiEmbedder{
		apiKey:     apiKey,
		model:      model,
		httpClient: newHTTPClient(),
	}
}

type geminiEmbedRequest struct {
	Model   string            `json:"model"`
	Content geminiEmbedContent `json:"content"`
}

type geminiEmbedContent struct {
	Parts []geminiEmbedPart `json:"parts"`
}

type geminiEmbedPart struct {
	Text string `json:"text"`
}

type geminiEmbedResponse struct {
	Embedding struct {
		Values []float32 `json:"values"`
	} `json:"embedding"`
	Error *struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error,omitempty"`
}

func (e *GeminiEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	reqBody := geminiEmbedRequest{
		Model: "models/" + e.model,
		Content: geminiEmbedContent{
			Parts: []geminiEmbedPart{{Text: text}},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal embed request: %w", err)
	}

	url := fmt.Sprintf(geminiEmbedURL, e.model, e.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("buat embed request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("kirim embed request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("baca embed response: %w", err)
	}

	var result geminiEmbedResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parse embed response: %w", err)
	}

	if result.Error != nil {
		return nil, fmt.Errorf("gemini embedding error %d: %s", result.Error.Code, result.Error.Message)
	}

	if len(result.Embedding.Values) == 0 {
		return nil, fmt.Errorf("embedding kosong untuk teks: %.50s...", text)
	}

	return result.Embedding.Values, nil
}

func (e *GeminiEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	return embedBatchSequential(ctx, texts, e.Embed)
}

func (e *GeminiEmbedder) Dimensions() int { return geminiEmbedDims }

// ── Ollama Embedder ─────────────────────────────────────────

const (
	ollamaEmbedDims = 768 // nomic-embed-text menghasilkan 768 dimensi
)

// OllamaEmbedder memanggil endpoint embedding Ollama lokal.
// Jalankan model embedding terlebih dahulu:
//   ollama pull nomic-embed-text
type OllamaEmbedder struct {
	address    string // contoh: "http://localhost:11434"
	model      string // contoh: "nomic-embed-text"
	httpClient *http.Client
}

// NewOllamaEmbedder membuat OllamaEmbedder baru.
func NewOllamaEmbedder(address, model string) *OllamaEmbedder {
	return &OllamaEmbedder{
		address:    address,
		model:      model,
		httpClient: newHTTPClient(),
	}
}

type ollamaEmbedRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type ollamaEmbedResponse struct {
	Embedding []float32 `json:"embedding"`
	Error     string    `json:"error,omitempty"`
}

func (e *OllamaEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	reqBody := ollamaEmbedRequest{
		Model:  e.model,
		Prompt: text,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal ollama embed request: %w", err)
	}

	url := e.address + "/api/embeddings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("buat ollama embed request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("kirim ollama embed request (pastikan Ollama berjalan): %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("baca ollama embed response: %w", err)
	}

	var result ollamaEmbedResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parse ollama embed response: %w", err)
	}

	if result.Error != "" {
		return nil, fmt.Errorf("ollama embedding error: %s", result.Error)
	}

	if len(result.Embedding) == 0 {
		return nil, fmt.Errorf("ollama embedding kosong — pastikan model '%s' sudah di-pull: ollama pull %s", e.model, e.model)
	}

	return result.Embedding, nil
}

func (e *OllamaEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	return embedBatchSequential(ctx, texts, e.Embed)
}

func (e *OllamaEmbedder) Dimensions() int { return ollamaEmbedDims }

// ── Factory ─────────────────────────────────────────────────

// NewEmbedder membuat Embedder yang sesuai berdasarkan provider.
//   provider "ollama" → OllamaEmbedder (pakai nomic-embed-text)
//   provider "gemini" → GeminiEmbedder (pakai text-embedding-004)
func NewEmbedder(provider, ollamaAddress, ollamaEmbedModel, geminiAPIKey, geminiEmbedModel string) Embedder {
	if provider == "gemini" {
		return NewGeminiEmbedder(geminiAPIKey, geminiEmbedModel)
	}
	// default: ollama
	return NewOllamaEmbedder(ollamaAddress, ollamaEmbedModel)
}
