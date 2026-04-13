// internal/llm/ollama.go
// Provider LLM menggunakan Ollama (model lokal) via Firebase Genkit.
//
// Konfigurasi di .env:
//   LLM_PROVIDER=ollama
//   OLLAMA_MODEL=llama3.2
//   OLLAMA_ADDRESS=http://localhost:11434

package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/ollama"
)

// OllamaProvider mengimplementasikan LLMProvider menggunakan Ollama.
type OllamaProvider struct {
	g         *genkit.Genkit
	modelName string // contoh: "llama3.2"
	address   string // contoh: "http://localhost:11434"
}

// NewOllamaProvider menginisialisasi Genkit dengan plugin Ollama.
func NewOllamaProvider(ctx context.Context, modelName, address string) (*OllamaProvider, error) {
	g := genkit.Init(ctx, genkit.WithPlugins(
		&ollama.Ollama{ServerAddress: address},
	))
	return &OllamaProvider{g: g, modelName: modelName, address: address}, nil
}

// Generate mengirim prompt ke Ollama dan mengembalikan teks respons.
func (p *OllamaProvider) Generate(ctx context.Context, prompt string) (string, error) {
	resp, err := genkit.Generate(ctx, p.g,
		ai.WithPrompt(prompt),
		ai.WithModelName("ollama/"+p.modelName),
	)
	if err != nil {
		return "", fmt.Errorf("ollama generate error: %w", err)
	}
	return strings.TrimSpace(resp.Text()), nil
}

// Name mengembalikan identifier provider ini.
func (p *OllamaProvider) Name() string {
	return fmt.Sprintf("ollama/%s", p.modelName)
}
