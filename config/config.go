// config/config.go
// Memuat konfigurasi dari environment variable atau nilai default.
//
// LLM_PROVIDER menentukan provider yang digunakan:
//   - "ollama"  → model lokal via Ollama
//   - "gemini"  → Google Gemini via API key
// Tambahkan case baru di internal/llm/factory.go untuk provider lain.

package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config menyimpan semua konfigurasi aplikasi.
type Config struct {
	// ── Provider selector ────────────────────────────────────
	LLMProvider string // "ollama" | "gemini" | ...

	// ── Ollama ───────────────────────────────────────────────
	OllamaModel   string
	OllamaAddress string

	// ── Gemini ───────────────────────────────────────────────
	GeminiModel  string
	GeminiAPIKey string

	// ── Embedding ────────────────────────────────────────────
	// Untuk Ollama: model embedding terpisah dari model LLM
	OllamaEmbedModel string // default: "nomic-embed-text"
	// Untuk Gemini: model embedding Gemini
	GeminiEmbedModel string // default: "text-embedding-004"

	// ── Qdrant ───────────────────────────────────────────────
	QdrantURL        string // default: "http://localhost:6333"
	QdrantCollection string // default: "products"

	// ── Database ─────────────────────────────────────────────
	MySQLDSN string

	// ── HTTP Server ──────────────────────────────────────────
	ServerPort     string // default: 8080
	AllowedOrigins string // CORS, default: "*"

	// ── Misc ─────────────────────────────────────────────────
	DataDir string
}

func Load() *Config {
	_ = godotenv.Load()

	// MySQL DSN
	host := getEnv("MYSQL_HOST", "localhost")
	port := getEnv("MYSQL_PORT", "3307")
	user := getEnv("MYSQL_USER", "root")
	password := getEnv("MYSQL_PASSWORD", "")
	database := getEnv("MYSQL_DATABASE", "chatbot_db")

	if _, err := strconv.Atoi(port); err != nil {
		port = "3307"
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true",
		user, password, host, port, database,
	)

	return &Config{
		LLMProvider: getEnv("LLM_PROVIDER", "ollama"),

		OllamaModel:   getEnv("OLLAMA_MODEL", "llama3.2"),
		OllamaAddress: getEnv("OLLAMA_ADDRESS", "http://localhost:11434"),

		GeminiModel:  getEnv("GEMINI_MODEL", "gemini-2.0-flash"),
		GeminiAPIKey: getEnv("GEMINI_API_KEY", ""),

		OllamaEmbedModel: getEnv("OLLAMA_EMBED_MODEL", "nomic-embed-text"),
		GeminiEmbedModel: getEnv("GEMINI_EMBED_MODEL", "text-embedding-004"),

		QdrantURL:        getEnv("QDRANT_URL", "http://localhost:6333"),
		QdrantCollection: getEnv("QDRANT_COLLECTION", "products"),

		MySQLDSN: dsn,

		ServerPort:     getEnv("SERVER_PORT", "8080"),
		AllowedOrigins: getEnv("ALLOWED_ORIGINS", "*"),

		DataDir: getEnv("DATA_DIR", "./data"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}