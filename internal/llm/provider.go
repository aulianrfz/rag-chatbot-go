// internal/llm/provider.go
// Mendefinisikan interface LLMProvider yang harus diimplementasikan
// oleh setiap provider LLM (Ollama, Gemini, OpenAI, dst.).
//
// Untuk menambah provider baru:
//   1. Buat file baru di internal/llm/, misal: openai.go
//   2. Implementasikan interface LLMProvider
//   3. Daftarkan di factory.go

package llm

import "context"

// LLMProvider adalah kontrak yang harus dipenuhi oleh semua provider LLM.
type LLMProvider interface {
	// Generate mengirim prompt ke LLM dan mengembalikan teks respons.
	Generate(ctx context.Context, prompt string) (string, error)

	// Name mengembalikan nama provider + model, untuk logging.
	Name() string
}