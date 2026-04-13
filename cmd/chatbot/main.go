// cmd/chatbot/main.go
// Entry point utama chatbot RAG.
// Provider LLM ditentukan oleh LLM_PROVIDER di .env — tidak ada
// kode provider di sini, semuanya di-abstract lewat llm.LLMProvider.
//
// Alur per pesan:
//   User Input
//       ↓
//   RAGEngine.GetContext()        ← cari data relevan dari JSON
//       ↓
//   Build prompt (system + context + history + input)
//       ↓
//   LLMProvider.Generate()        ← Ollama / Gemini / dst.
//       ↓
//   Tampilkan respons + simpan ke MySQL

package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"rag-chatbot-go/config"
	"rag-chatbot-go/internal/llm"
	"rag-chatbot-go/internal/memory"
	"rag-chatbot-go/internal/rag"
)

// ─────────────────────────────────────────────
// SYSTEM PROMPT TEMPLATE
// ─────────────────────────────────────────────
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

// ─────────────────────────────────────────────
// CHAT SESSION
// ─────────────────────────────────────────────

// historyEntry menyimpan satu giliran percakapan.
type historyEntry struct {
	Role    string // "human" | "ai"
	Content string
}

// ChatSession memegang semua state percakapan satu sesi.
type ChatSession struct {
	sessionID string
	history   []historyEntry
	store     *memory.Store   // nil jika DB tidak tersambung
	ragEngine *rag.Engine
	provider  llm.LLMProvider // ← abstraksi: tidak peduli Ollama atau Gemini
}

// Chat memproses satu pesan user dan mengembalikan respons bot.
func (cs *ChatSession) Chat(ctx context.Context, userInput string) (string, error) {
	// 1. RETRIEVE — semantic search via Gemini embedding + Qdrant
	ragContext := cs.ragEngine.GetContext(ctx, userInput, 3)

	// 2. AUGMENT — bangun prompt lengkap (system + context + history + input)
	systemPrompt := fmt.Sprintf(systemPromptTemplate, ragContext)

	// Sliding window: gunakan hanya N item terakhir dari history (3 turn = 6 item)
	const maxHistoryItems = 6
	startIdx := 0
	if len(cs.history) > maxHistoryItems {
		startIdx = len(cs.history) - maxHistoryItems
	}

	var sb strings.Builder
	for _, h := range cs.history[startIdx:] {
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

	// 3. GENERATE — delegasikan ke provider aktif
	response, err := cs.provider.Generate(ctx, fullPrompt)
	if err != nil {
		return "", err
	}

	// 4. STORE — simpan ke in-memory history
	cs.history = append(cs.history,
		historyEntry{Role: "human", Content: userInput},
		historyEntry{Role: "ai", Content: response},
	)

	// 5. PERSIST — simpan ke MySQL (jika tersambung)
	if cs.store != nil {
		_ = cs.store.Save(cs.sessionID, "human", userInput)
		_ = cs.store.Save(cs.sessionID, "ai", response)
	}

	return response, nil
}

// ─────────────────────────────────────────────
// MAIN
// ─────────────────────────────────────────────

func main() {
	fmt.Println(strings.Repeat("=", 55))
	fmt.Println("  🛒  RAG Chatbot — Asisten Toko Online (Go)")
	fmt.Println(strings.Repeat("=", 55))

	ctx := context.Background()
	cfg := config.Load()

	// ── Inisialisasi LLM Provider (dari config) ──────────────
	provider, err := llm.New(ctx, cfg)
	if err != nil {
		log.Fatalf("gagal init LLM provider: %v", err)
	}
	fmt.Printf("🤖 Provider: %s\n", provider.Name())

	// ── Inisialisasi RAG Engine ───────────────────────────────
	ragEngine, err := rag.New(rag.EngineConfig{
		DataDir:          cfg.DataDir,
		LLMProvider:      cfg.LLMProvider,
		OllamaAddress:    cfg.OllamaAddress,
		OllamaEmbedModel: cfg.OllamaEmbedModel,
		GeminiAPIKey:     cfg.GeminiAPIKey,
		GeminiEmbedModel: cfg.GeminiEmbedModel,
		QdrantURL:        cfg.QdrantURL,
		QdrantCollection: cfg.QdrantCollection,
	})
	if err != nil {
		log.Fatalf("gagal init RAG engine: %v", err)
	}

	// ── Inisialisasi MySQL (opsional) ─────────────────────────
	var store *memory.Store
	store, err = memory.New(cfg.MySQLDSN)
	if err != nil {
		fmt.Printf("⚠️  MySQL tidak tersambung: %v\n", err)
		fmt.Println("   History tidak akan disimpan ke database.\n")
	} else {
		defer store.Close()
		fmt.Println("✅ Database siap.")
	}

	fmt.Println("💬 Chatbot siap!\n")

	// ── Session ───────────────────────────────────────────────
	const sessionID = "session_001"
	session := &ChatSession{
		sessionID: sessionID,
		ragEngine: ragEngine,
		store:     store,
		provider:  provider,
	}

	// Muat history dari MySQL jika tersambung
	if store != nil {
		rows, err := store.LoadHistory(sessionID, 10)
		if err == nil && len(rows) > 0 {
			for _, r := range rows {
				session.history = append(session.history, historyEntry{
					Role:    r.Role,
					Content: r.Content,
				})
			}
			fmt.Printf("📜 Memuat %d pesan dari history sesi '%s'\n\n", len(rows), sessionID)
		}
	}

	// Statistik data
	produk, transaksi, dokumen := ragEngine.Stats()
	fmt.Printf("📊 Data: %d produk | %d transaksi | %d dokumen index\n\n",
		produk, transaksi, dokumen)

	fmt.Println("💡 Contoh pertanyaan:")
	fmt.Println("   - Produk apa yang paling sering dibeli?")
	fmt.Println("   - Berapa harga laptop ASUS?")
	fmt.Println("   - Apakah mouse Logitech masih ada stoknya?")
	fmt.Println()
	fmt.Println("Ketik 'keluar' untuk berhenti | 'reset' untuk hapus history")
	fmt.Println(strings.Repeat("-", 55) + "\n")

	// ── Loop percakapan ───────────────────────────────────────
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("Kamu: ")
		if !scanner.Scan() {
			break
		}
		userInput := strings.TrimSpace(scanner.Text())
		if userInput == "" {
			continue
		}

		switch strings.ToLower(userInput) {
		case "keluar", "exit", "quit":
			fmt.Println("Bot: Terima kasih, sampai jumpa! 👋")
			return
		case "reset":
			session.history = nil
			fmt.Println("Bot: Riwayat percakapan direset.\n")
			continue
		}

		response, err := session.Chat(ctx, userInput)
		if err != nil {
			fmt.Printf("❌ Error: %v\n\n", err)
			continue
		}

		fmt.Printf("\nBot: %s\n\n", response)
		fmt.Println(strings.Repeat("-", 55))
	}
}