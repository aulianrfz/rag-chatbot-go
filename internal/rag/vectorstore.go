// internal/rag/vectorstore.go
// Client Qdrant untuk menyimpan dan mencari vektor dokumen.
// Berkomunikasi via Qdrant REST API (tidak perlu gRPC).
//
// Docs: https://qdrant.tech/documentation/interfaces/

package rag

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// VectorStore mengelola koneksi ke Qdrant.
type VectorStore struct {
	baseURL    string
	collection string
	httpClient *http.Client
}

// NewVectorStore membuat VectorStore baru.
func NewVectorStore(qdrantURL, collection string) *VectorStore {
	return &VectorStore{
		baseURL:    qdrantURL,
		collection: collection,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// ── Qdrant REST structs ─────────────────────────────────────

type qdrantCreateCollection struct {
	Vectors qdrantVectorsConfig `json:"vectors"`
}

type qdrantVectorsConfig struct {
	Size     int    `json:"size"`
	Distance string `json:"distance"`
}

type qdrantUpsertRequest struct {
	Points []qdrantPoint `json:"points"`
}

type qdrantPoint struct {
	ID      string         `json:"id"`
	Vector  []float32      `json:"vector"`
	Payload map[string]any `json:"payload"`
}

type qdrantSearchRequest struct {
	Vector         []float32 `json:"vector"`
	Limit          int       `json:"limit"`
	WithPayload    bool      `json:"with_payload"`
	ScoreThreshold float64   `json:"score_threshold"`
}

type qdrantSearchResponse struct {
	Result []struct {
		ID      string         `json:"id"`
		Score   float64        `json:"score"`
		Payload map[string]any `json:"payload"`
	} `json:"result"`
	Status string `json:"status"`
}

type qdrantGenericResponse struct {
	Status string `json:"status"`
	Result any    `json:"result"`
}

// ── SearchResult ────────────────────────────────────────────

// SearchResult adalah hasil pencarian dari vector store.
type SearchResult struct {
	ID      string
	Content string
	Score   float64
}

// ── Public methods ──────────────────────────────────────────

// EnsureCollection membuat collection jika belum ada.
// vectorSize harus sesuai dengan dimensi model embedding yang dipakai
// (text-embedding-004 menghasilkan 768 dimensi).
func (vs *VectorStore) EnsureCollection(ctx context.Context, vectorSize int) error {
	// Cek apakah collection sudah ada
	url := fmt.Sprintf("%s/collections/%s", vs.baseURL, vs.collection)
	resp, err := vs.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("cek collection: %w", err)
	}
	defer resp.Body.Close()

	// Collection sudah ada
	if resp.StatusCode == http.StatusOK {
		return nil
	}

	// Buat collection baru
	body := qdrantCreateCollection{
		Vectors: qdrantVectorsConfig{
			Size:     vectorSize,
			Distance: "Cosine",
		},
	}
	createResp, err := vs.doJSONRequest(ctx, http.MethodPut, url, body)
	if err != nil {
		return fmt.Errorf("buat collection: %w", err)
	}
	defer createResp.Body.Close()

	if createResp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(createResp.Body)
		return fmt.Errorf("buat collection gagal (status %d): %s", createResp.StatusCode, raw)
	}

	return nil
}

// Upsert menyimpan dokumen beserta vektornya ke Qdrant.
// Menggunakan upsert sehingga aman dipanggil ulang (idempotent).
// ID dokumen dikonversi ke UUID format karena Qdrant hanya menerima UUID atau uint64.
// ID asli tetap disimpan di payload sebagai "doc_id" untuk referensi.
func (vs *VectorStore) Upsert(ctx context.Context, docs []Document) error {
	if len(docs) == 0 {
		return nil
	}

	points := make([]qdrantPoint, len(docs))
	for i, doc := range docs {
		points[i] = qdrantPoint{
			ID:     toUUID(doc.ID), // konversi ke UUID deterministik
			Vector: doc.Embedding,
			Payload: map[string]any{
				"content": doc.Content,
				"doc_id":  doc.ID, // simpan ID asli untuk debugging
			},
		}
	}

	url := fmt.Sprintf("%s/collections/%s/points?wait=true", vs.baseURL, vs.collection)
	resp, err := vs.doJSONRequest(ctx, http.MethodPut, url, qdrantUpsertRequest{Points: points})
	if err != nil {
		return fmt.Errorf("upsert points: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upsert gagal (status %d): %s", resp.StatusCode, raw)
	}

	return nil
}

// Search mencari topK dokumen paling relevan berdasarkan vektor query.
func (vs *VectorStore) Search(ctx context.Context, queryVec []float32, topK int, scoreThreshold float64) ([]SearchResult, error) {
	url := fmt.Sprintf("%s/collections/%s/points/search", vs.baseURL, vs.collection)

	body := qdrantSearchRequest{
		Vector:         queryVec,
		Limit:          topK,
		WithPayload:    true,
		ScoreThreshold: scoreThreshold,
	}

	resp, err := vs.doJSONRequest(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("baca search response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search gagal (status %d): %s", resp.StatusCode, raw)
	}

	var result qdrantSearchResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parse search response: %w", err)
	}

	searchResults := make([]SearchResult, 0, len(result.Result))
	for _, hit := range result.Result {
		content, _ := hit.Payload["content"].(string)
		searchResults = append(searchResults, SearchResult{
			ID:      hit.ID,
			Content: content,
			Score:   hit.Score,
		})
	}

	return searchResults, nil
}

// CollectionExists mengecek apakah collection sudah ada di Qdrant.
func (vs *VectorStore) CollectionExists(ctx context.Context) (bool, error) {
	url := fmt.Sprintf("%s/collections/%s", vs.baseURL, vs.collection)
	resp, err := vs.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK, nil
}

// ── HTTP helpers ────────────────────────────────────────────

func (vs *VectorStore) doJSONRequest(ctx context.Context, method, url string, body any) (*http.Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return vs.doRequest(ctx, method, url, bytes.NewReader(data))
}

func (vs *VectorStore) doRequest(ctx context.Context, method, url string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return vs.httpClient.Do(req)
}

// ── ID helpers ──────────────────────────────────────────────

// toUUID mengkonversi arbitrary string ID ke format UUID v4-like yang deterministik.
// Qdrant hanya menerima UUID string atau uint64 sebagai point ID.
// Kita pakai SHA-256 hash lalu format sebagai UUID agar:
//   - Deterministik: ID yang sama → UUID yang sama (idempotent upsert)
//   - Unik: collision probability sangat rendah
func toUUID(id string) string {
	h := sha256.Sum256([]byte(id))
	// Format 16 byte pertama sebagai UUID: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		h[0:4],
		h[4:6],
		h[6:8],
		h[8:10],
		h[10:16],
	)
}
