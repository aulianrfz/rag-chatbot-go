# 🤖 RAG Chatbot Go — Backend API

Backend REST API untuk chatbot asisten belanja TokoKu, dibangun menggunakan Go dengan pendekatan RAG (Retrieval-Augmented Generation). Sistem ini mendukung beberapa LLM provider seperti Ollama dan Google Gemini, serta penyimpanan riwayat chat menggunakan MySQL.

---

## ✨ Features

-  **RAG Engine** — pencarian dokumen berbasis keyword scoring (TF-like) dari data produk & transaksi
-  **Multi LLM Provider** — dukung **Ollama** (lokal) dan **Google Gemini** (cloud), mudah diperluas
-  **Multi-session** — setiap sesi chat dikelola secara independen
-  **Persistensi MySQL (opsional)** — riwayat chat disimpan ke database; berjalan normal tanpa DB

---

## 🛠️ Tech Stack

| Komponen | Teknologi |
|---|---|
| Language | Go 1.25 |
| LLM (lokal) | [Ollama](https://ollama.com/) |
| LLM (cloud) | [Google Gemini](https://ai.google.dev/) via `google.golang.org/genai` |
| Database | MySQL (opsional) |
| HTTP | `net/http` standar library |
| Config | `github.com/joho/godotenv` |

## 🚀 Getting Started

### Prerequisites

- [Go](https://go.dev/dl/) 1.21+
- [Ollama](https://ollama.com/) (jika menggunakan provider lokal)
- MySQL (opsional, untuk persistensi riwayat chat)

### Installation

```bash
go mod tidy
```

### Konfigurasi

Salin file `.env` dan sesuaikan:

```bash
cp .env .env.local
```

Edit sesuai kebutuhan:

```env
# ── LLM Provider ─────────────────────────────
# Pilihan: ollama | gemini
LLM_PROVIDER=ollama

# ── Ollama (aktif jika LLM_PROVIDER=ollama) ──
OLLAMA_MODEL=llama3.2
OLLAMA_ADDRESS=http://localhost:11434

# ── Gemini (aktif jika LLM_PROVIDER=gemini) ──
GEMINI_MODEL=gemini-2.0-flash
GEMINI_API_KEY=your_api_key_here

# ── MySQL (opsional) ─────────────────────────
MYSQL_HOST=localhost
MYSQL_PORT=3307
MYSQL_USER=root
MYSQL_PASSWORD=
MYSQL_DATABASE=chatbot_go

# ── HTTP Server ───────────────────────────────
SERVER_PORT=8080
ALLOWED_ORIGINS=*

# ── Data ─────────────────────────────────────
DATA_DIR=./data
```

### Menjalankan Server

```bash
# HTTP API server
go run ./cmd/api

# Atau mode CLI (terminal chatbot)
go run ./cmd/chatbot
```

Server berjalan di `http://localhost:8080`.

---

## 🔍 Cara Kerja RAG

1. **Indexing** — saat server start, `products.json` dan `purchases.json` dikonversi menjadi dokumen teks terstruktur dan diindeks ke dalam memori.
2. **Retrieval** — setiap pesan user di-tokenize, lalu dicocokkan dengan dokumen menggunakan keyword scoring. Top-K dokumen paling relevan diambil sebagai konteks.
3. **Generation** — konteks yang ditemukan digabungkan dengan prompt dan dikirim ke LLM provider untuk menghasilkan jawaban.

---

## 🔌Supported LLM Provider
- Ollama → model lokal, tanpa biaya API
- Google Gemini → model cloud dengan API key

Struktur provider dibuat modular sehingga mudah menambah provider baru seperti OpenAI, Claude, dan lainnya.

---

## 🗄️ Database (Opsional)

MySQL digunakan untuk mempersist riwayat chat antar restart server. Tabel `chat_history` dibuat otomatis saat pertama kali koneksi berhasil.

Jika MySQL tidak tersambung, server tetap berjalan normal — riwayat chat hanya disimpan di memori (hilang saat server restart).

**Skema tabel:**
```sql
CREATE TABLE IF NOT EXISTS chat_history (
    id         INT AUTO_INCREMENT PRIMARY KEY,
    session_id VARCHAR(100),
    role       VARCHAR(20),
    message    TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

---

## 🔗 Related Repository

Frontend Mobile App:
https://github.com/aulianrfz/mini-chatbot

---

## 👨‍💻 Author

Aulia Nurul F
GitHub: https://github.com/aulianrfz

---

## 📄 License

Private Project — All Rights Reserved.
