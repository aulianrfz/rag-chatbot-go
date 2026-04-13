// cmd/api/main.go
// Entry point HTTP API server.
// Jalankan: go run ./cmd/api
//
// API Endpoints:
//   POST   /api/chat                        → kirim pesan
//   GET    /api/chat/{session_id}/history   → riwayat percakapan
//   DELETE /api/chat/{session_id}           → hapus session
//   GET    /api/health                      → status server

package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"rag-chatbot-go/config"
	"rag-chatbot-go/internal/api"
	"rag-chatbot-go/internal/chat"
	"rag-chatbot-go/internal/llm"
	"rag-chatbot-go/internal/memory"
	"rag-chatbot-go/internal/rag"
)

func main() {
	fmt.Println("╔══════════════════════════════════════════════════╗")
	fmt.Println("║    🛒  RAG Chatbot API — Toko Elektronik         ║")
	fmt.Println("╚══════════════════════════════════════════════════╝")

	cfg := config.Load()
	ctx := context.Background()

	// ── LLM Provider ─────────────────────────────────────────
	provider, err := llm.New(ctx, cfg)
	if err != nil {
		log.Fatalf("gagal init LLM provider: %v", err)
	}
	fmt.Printf("🤖 Provider  : %s\n", provider.Name())

	// ── RAG Engine ────────────────────────────────────────────
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
	produk, transaksi, dokumen := ragEngine.Stats()
	fmt.Printf("📊 Data      : %d produk | %d transaksi | %d dokumen\n",
		produk, transaksi, dokumen)

	// ── MySQL (opsional) ──────────────────────────────────────
	var store *memory.Store
	store, err = memory.New(cfg.MySQLDSN)
	if err != nil {
		fmt.Printf("⚠️  MySQL tidak tersambung: %v\n", err)
		fmt.Println("   History tidak akan dipersist ke database.")
	} else {
		defer store.Close()
		fmt.Println("✅ Database  : tersambung")
	}

	// ── Session Manager ───────────────────────────────────────
	manager := chat.NewManager(ragEngine, provider, store)

	// ── HTTP Server ───────────────────────────────────────────
	handler := api.NewHandler(manager, ragEngine)
	router := api.NewRouter(handler, cfg.AllowedOrigins)

	addr := ":" + cfg.ServerPort
	server := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second, // LLM bisa lambat
		IdleTimeout:  60 * time.Second,
	}

	// ── Graceful shutdown ─────────────────────────────────────
	shutdownCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		fmt.Printf("\n🚀 Server berjalan di http://localhost%s\n", addr)
		fmt.Println("─────────────────────────────────────────────────")
		fmt.Printf("  POST   http://localhost%s/api/chat\n", addr)
		fmt.Printf("  GET    http://localhost%s/api/chat/:id/history\n", addr)
		fmt.Printf("  DELETE http://localhost%s/api/chat/:id\n", addr)
		fmt.Printf("  GET    http://localhost%s/api/health\n", addr)
		fmt.Println("─────────────────────────────────────────────────")
		fmt.Println("Tekan Ctrl+C untuk berhenti\n")

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-shutdownCtx.Done()
	fmt.Println("\n⏳ Mematikan server...")

	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutCtx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
	fmt.Println("👋 Server berhenti.")
}