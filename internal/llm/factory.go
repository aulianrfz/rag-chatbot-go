// internal/llm/factory.go
// Factory: membaca LLM_PROVIDER dari config dan mengembalikan
// implementasi LLMProvider yang sesuai.
//
// Cara menambah provider baru:
//   1. Buat file provider baru (misal: openai.go) yang implements LLMProvider
//   2. Tambahkan case baru di switch di bawah
//   3. Tambahkan konfigurasi yang dibutuhkan ke config/config.go dan .env.example

package llm

import (
	"context"
	"fmt"

	"rag-chatbot-go/config"
)

// New membaca config dan mengembalikan LLMProvider yang sesuai.
func New(ctx context.Context, cfg *config.Config) (LLMProvider, error) {
	switch cfg.LLMProvider {
	case "ollama":
		return NewOllamaProvider(ctx, cfg.OllamaModel, cfg.OllamaAddress)

	case "gemini":
		if cfg.GeminiAPIKey == "" {
			return nil, fmt.Errorf("GEMINI_API_KEY wajib diisi untuk provider 'gemini'")
		}
		return NewGeminiProvider(ctx, cfg.GeminiModel, cfg.GeminiAPIKey)

	default:
		return nil, fmt.Errorf(
			"provider LLM '%s' tidak dikenal. Pilihan yang tersedia: ollama, gemini",
			cfg.LLMProvider,
		)
	}
}