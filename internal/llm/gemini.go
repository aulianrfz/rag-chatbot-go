// internal/llm/gemini.go
// Provider LLM menggunakan Google Gemini via Firebase Genkit.
//
// Konfigurasi di .env:
//   LLM_PROVIDER=gemini
//   GEMINI_MODEL=gemini-2.0-flash        # atau gemini-1.5-pro, gemini-1.5-flash
//   GEMINI_API_KEY=AIza...

package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/googlegenai"
)

// GeminiProvider mengimplementasikan LLMProvider menggunakan Google Gemini.
type GeminiProvider struct {
	g         *genkit.Genkit
	modelName string // contoh: "gemini-2.0-flash"
}

// NewGeminiProvider menginisialisasi Genkit dengan plugin Google GenAI.
func NewGeminiProvider(ctx context.Context, modelName, apiKey string) (*GeminiProvider, error) {
	g := genkit.Init(ctx, genkit.WithPlugins(
		&googlegenai.GoogleAI{APIKey: apiKey},
	))
	return &GeminiProvider{g: g, modelName: modelName}, nil
}

// Generate mengirim prompt ke Gemini dan mengembalikan teks respons.
func (p *GeminiProvider) Generate(ctx context.Context, prompt string) (string, error) {
	resp, err := genkit.Generate(ctx, p.g,
		ai.WithPrompt(prompt),
		ai.WithModelName("googleai/"+p.modelName),
	)
	if err != nil {
		return "", fmt.Errorf("gemini generate error: %w", err)
	}
	return strings.TrimSpace(resp.Text()), nil
}

// Name mengembalikan identifier provider ini.
func (p *GeminiProvider) Name() string {
	return fmt.Sprintf("gemini/%s", p.modelName)
}
