// internal/chat/session.go
// Core logic percakapan: RAG + LLM + history.
// Dipakai oleh CLI (cmd/chatbot) maupun HTTP API (cmd/api).

package chat

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"rag-chatbot-go/internal/llm"
	"rag-chatbot-go/internal/memory"
	"rag-chatbot-go/internal/rag"
)

const systemPromptTemplate = `Kamu adalah asisten toko elektronik bernama Kak Toko.

DATA TOKO:
%s

ATURAN:
1. Jika DATA TOKO berisi "TIDAK_ADA_PRODUK" → jawab hanya:
   "Tidak ada produk di rentang harga tersebut."
   Tidak perlu kalimat lain.

2. Jawab HANYA dari DATA TOKO. Jangan mengarang produk atau harga.

3. Jika pertanyaan tidak relevan dengan toko elektronik (misal: makanan, pakaian):
   Jawab: "Kami hanya menjual elektronik, tidak menjual [sebut itemnya]."

4. Jika ada typo (leptop=laptop, mkouse=mouse, keybaord=keyboard):
   Koreksi diam-diam, lalu jawab. Tidak perlu sebut koreksinya kecuali ambigu.

FORMAT JAWABAN:
- Singkat dan langsung. Tidak perlu basa-basi pembuka atau penutup.
- Produk spesifik → nama, harga, stok, 1 kalimat kegunaan.
- Rekomendasi → pilih 1-2 yang paling cocok saja, jelaskan kenapa.
- Daftar terlaris → nomor urut, maks 3.
- Harga: Rp 7.500.000 (bukan 7500000).`

// HistoryEntry menyimpan satu giliran percakapan.
type HistoryEntry struct {
	Role    string `json:"role"`    // "human" | "ai"
	Content string `json:"content"`
}

// Session memegang state percakapan satu sesi.
// Thread-safe: aman diakses bersamaan dari banyak goroutine HTTP.
type Session struct {
	mu        sync.Mutex
	SessionID string
	history   []HistoryEntry
	store     *memory.Store   // nil jika DB tidak tersambung
	ragEngine *rag.Engine
	provider  llm.LLMProvider
}

// NewSession membuat session baru dan memuat history dari DB jika tersedia.
func NewSession(
	sessionID string,
	ragEngine *rag.Engine,
	provider llm.LLMProvider,
	store *memory.Store,
) *Session {
	s := &Session{
		SessionID: sessionID,
		ragEngine: ragEngine,
		provider:  provider,
		store:     store,
	}

	if store != nil {
		rows, err := store.LoadHistory(sessionID, 10)
		if err == nil {
			for _, r := range rows {
				s.history = append(s.history, HistoryEntry{
					Role:    r.Role,
					Content: r.Content,
				})
			}
		}
	}
	return s
}

// Chat memproses satu pesan dan mengembalikan respons bot.
func (s *Session) Chat(ctx context.Context, userInput string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. RETRIEVE — semantic search via Gemini embedding + Qdrant
	ragContext := s.ragEngine.GetContext(ctx, userInput, 3)

	// 2. AUGMENT
	systemPrompt := fmt.Sprintf(systemPromptTemplate, ragContext)

	// Sliding window: gunakan hanya N item terakhir dari history (3 turn = 6 item)
	const maxHistoryItems = 6
	startIdx := 0
	if len(s.history) > maxHistoryItems {
		startIdx = len(s.history) - maxHistoryItems
	}

	var sb strings.Builder
	for _, h := range s.history[startIdx:] {
		switch h.Role {
		case "human":
			sb.WriteString("User: " + h.Content + "\n")
		case "ai":
			sb.WriteString("Kak Toko: " + h.Content + "\n")
		}
	}
	sb.WriteString("User: " + userInput + "\n")
	sb.WriteString("Kak Toko:")

	fullPrompt := systemPrompt + "\n\nRIWAYAT PERCAKAPAN:\n" + sb.String()

	// 3. GENERATE
	response, err := s.provider.Generate(ctx, fullPrompt)
	if err != nil {
		return "", err
	}

	// 4. STORE in-memory
	s.history = append(s.history,
		HistoryEntry{Role: "human", Content: userInput},
		HistoryEntry{Role: "ai", Content: response},
	)

	// 5. PERSIST ke DB
	if s.store != nil {
		_ = s.store.Save(s.SessionID, "human", userInput)
		_ = s.store.Save(s.SessionID, "ai", response)
	}

	return response, nil
}

// History mengembalikan salinan history saat ini (thread-safe).
func (s *Session) History() []HistoryEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	cp := make([]HistoryEntry, len(s.history))
	copy(cp, s.history)
	return cp
}

// Reset menghapus history in-memory sesi ini.
func (s *Session) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = nil
}