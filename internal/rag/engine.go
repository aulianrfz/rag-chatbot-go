// internal/rag/engine.go
// RAG Engine: memuat data produk & transaksi, menghasilkan embedding
// via Gemini API, menyimpan ke Qdrant, dan melakukan semantic search
// untuk mengambil dokumen yang paling relevan dengan query user.
//
// Alur indexing:
//   loadProducts + loadPurchases
//       ↓
//   buildDocuments()          ← buat teks dokumen dari data JSON
//       ↓
//   embedder.EmbedBatch()     ← ubah setiap dokumen ke vektor float32
//       ↓
//   vectorStore.Upsert()      ← simpan vektor ke Qdrant
//
// Alur retrieval:
//   GetContext(query)
//       ↓
//   embedder.Embed(query)     ← ubah query ke vektor
//       ↓
//   vectorStore.Search()      ← cosine similarity search di Qdrant
//       ↓
//   gabungkan teks dokumen teratas → kembalikan sebagai context

package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Engine menyimpan index dokumen dan klien untuk embedding + vector store.
type Engine struct {
	products  []Product
	purchases []Purchase
	documents []Document // dokumen mentah (tanpa vektor, untuk stats)

	embedder    Embedder
	vectorStore *VectorStore
}

// EngineConfig untuk inisialisasi Engine dengan embedding + vector store.
type EngineConfig struct {
	DataDir          string
	LLMProvider      string // "ollama" | "gemini"
	OllamaAddress    string
	OllamaEmbedModel string
	GeminiAPIKey     string
	GeminiEmbedModel string
	QdrantURL        string
	QdrantCollection string
}

// New memuat data, membangun embedding, dan menyimpannya ke Qdrant.
func New(cfg EngineConfig) (*Engine, error) {
	ctx := context.Background()

	// Pilih dimensi sesuai provider
	embedder := NewEmbedder(
		cfg.LLMProvider,
		cfg.OllamaAddress,
		cfg.OllamaEmbedModel,
		cfg.GeminiAPIKey,
		cfg.GeminiEmbedModel,
	)

	e := &Engine{
		embedder:    embedder,
		vectorStore: NewVectorStore(cfg.QdrantURL, cfg.QdrantCollection),
	}

	// 1. Muat data dari JSON
	if err := e.loadProducts(filepath.Join(cfg.DataDir, "products.json")); err != nil {
		return nil, fmt.Errorf("gagal muat products: %w", err)
	}
	if err := e.loadPurchases(filepath.Join(cfg.DataDir, "purchases.json")); err != nil {
		return nil, fmt.Errorf("gagal muat purchases: %w", err)
	}

	// 2. Buat dokumen teks dari data
	e.documents = e.buildDocuments()

	// 3. Pastikan collection Qdrant ada (dimensi dari embedder yang aktif)
	if err := e.vectorStore.EnsureCollection(ctx, e.embedder.Dimensions()); err != nil {
		return nil, fmt.Errorf("gagal inisialisasi Qdrant collection: %w", err)
	}

	// 4. Index dokumen ke Qdrant (embed + upsert)
	if err := e.indexDocuments(ctx); err != nil {
		return nil, fmt.Errorf("gagal index dokumen: %w", err)
	}

	return e, nil
}

// ── Data loading ─────────────────────────────────────────────

func (e *Engine) loadProducts(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &e.products)
}

func (e *Engine) loadPurchases(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &e.purchases)
}

// ── Document building ─────────────────────────────────────────

// buildDocuments mengubah produk dan data penjualan menjadi slice Document.
func (e *Engine) buildDocuments() []Document {
	var docs []Document

	// Dokumen per produk
	for _, p := range e.products {
		content := fmt.Sprintf(
			"Produk: %s | Kategori: %s | Harga: Rp %s | Stok: %d unit | "+
				"Deskripsi: %s | Tags: %s",
			p.Nama,
			p.Kategori,
			formatRupiah(p.Harga),
			p.Stok,
			p.Deskripsi,
			strings.Join(p.Tags, ", "),
		)
		docs = append(docs, Document{
			ID:      "prod_" + p.ID,
			Content: content,
		})
	}

	// Dokumen ringkasan penjualan per produk
	salesMap := e.aggregateSales()
	for prodID, qty := range salesMap {
		prodName := e.productName(prodID)
		content := fmt.Sprintf(
			"Penjualan: %s (ID: %s) terjual %d unit total.",
			prodName, prodID, qty,
		)
		docs = append(docs, Document{
			ID:      "sale_" + prodID,
			Content: content,
		})
	}

	// Dokumen terlaris (top 3)
	topContent := e.buildTopSalesDoc(salesMap)
	docs = append(docs, Document{
		ID:      "top_sales",
		Content: topContent,
	})

	return docs
}

// ── Indexing ──────────────────────────────────────────────────

// indexDocuments menghasilkan embedding untuk setiap dokumen dan menyimpannya ke Qdrant.
func (e *Engine) indexDocuments(ctx context.Context) error {
	if len(e.documents) == 0 {
		return nil
	}

	fmt.Printf("⏳ Menghasilkan embedding untuk %d dokumen...\n", len(e.documents))

	// Kumpulkan teks semua dokumen
	texts := make([]string, len(e.documents))
	for i, doc := range e.documents {
		texts[i] = doc.Content
	}

	// Embed semua dokumen (sequential dengan rate-limit handling)
	vectors, err := e.embedder.EmbedBatch(ctx, texts)
	if err != nil {
		return fmt.Errorf("embed batch dokumen: %w", err)
	}

	// Pasangkan vektor ke dokumen
	docsWithVectors := make([]Document, len(e.documents))
	for i, doc := range e.documents {
		docsWithVectors[i] = doc
		docsWithVectors[i].Embedding = vectors[i]
	}

	// Upsert ke Qdrant
	if err := e.vectorStore.Upsert(ctx, docsWithVectors); err != nil {
		return fmt.Errorf("upsert ke Qdrant: %w", err)
	}

	fmt.Printf("✅ %d dokumen berhasil diindex ke Qdrant.\n", len(docsWithVectors))
	return nil
}

// ── Retrieval ─────────────────────────────────────────────────

// GetContext mencari topK dokumen paling relevan untuk query menggunakan
// semantic search (cosine similarity di Qdrant), lalu menggabungkan
// teks dokumen tersebut menjadi satu string konteks.
func (e *Engine) GetContext(ctx context.Context, query string, topK int) string {
	// 1. Embed query
	queryVec, err := e.embedder.Embed(ctx, query)
	if err != nil {
		fmt.Printf("⚠️  gagal embed query: %v\n", err)
		return "TIDAK_ADA_DATA"
	}

	// 2. Semantic search di Qdrant
	// SESUDAH
	// Threshold 0.5: dokumen dengan kemiripan < 50% diabaikan.
	// Query tidak relevan (misal "jua makanan?") akan menghasilkan 0 dokumen.
	const scoreThreshold = 0.5
	results, err := e.vectorStore.Search(ctx, queryVec, topK, scoreThreshold)
	if err != nil {
		fmt.Printf("⚠️  gagal vector search: %v\n", err)
		return "TIDAK_ADA_DATA"
	}

	if len(results) == 0 {
		return "TIDAK_ADA_PRODUK"
	}

	// 3. Gabungkan konten dokumen teratas
	var parts []string
	for _, r := range results {
		if r.Content != "" {
			parts = append(parts, r.Content)
		}
	}

	if len(parts) == 0 {
		return "TIDAK_ADA_PRODUK"
	}

	return strings.Join(parts, "\n\n")
}

// Stats mengembalikan statistik data yang dimuat.
func (e *Engine) Stats() (totalProduk, totalTransaksi, totalDokumen int) {
	return len(e.products), len(e.purchases), len(e.documents)
}

// ── Helpers ───────────────────────────────────────────────────

func (e *Engine) aggregateSales() map[string]int {
	m := make(map[string]int)
	for _, t := range e.purchases {
		m[t.ProdukID] += t.Jumlah
	}
	return m
}

func (e *Engine) buildTopSalesDoc(salesMap map[string]int) string {
	type entry struct {
		id  string
		qty int
	}
	var entries []entry
	for id, qty := range salesMap {
		entries = append(entries, entry{id, qty})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].qty > entries[j].qty
	})

	var sb strings.Builder
	sb.WriteString("Produk terlaris: ")
	limit := int(math.Min(3, float64(len(entries))))
	for i := 0; i < limit; i++ {
		sb.WriteString(fmt.Sprintf("%d. %s (%d unit)", i+1,
			e.productName(entries[i].id), entries[i].qty))
		if i < limit-1 {
			sb.WriteString("; ")
		}
	}
	return sb.String()
}

func (e *Engine) productName(id string) string {
	for _, p := range e.products {
		if p.ID == id {
			return p.Nama
		}
	}
	return id
}

func formatRupiah(n int64) string {
	s := fmt.Sprintf("%d", n)
	var result []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, '.')
		}
		result = append(result, byte(c))
	}
	return string(result)
}
