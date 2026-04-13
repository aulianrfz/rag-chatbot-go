// internal/api/router.go
// Mendaftarkan semua route dan middleware ke http.ServeMux.
//
// Routes:
//   POST   /api/chat
//   GET    /api/chat/{session_id}/history
//   DELETE /api/chat/{session_id}
//   GET    /api/health

package api

import (
	"log"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
)

// NewRouter membuat http.ServeMux dengan semua route terdaftar.
func NewRouter(h *Handler, allowedOrigins string) http.Handler {
	mux := http.NewServeMux()

	// ── Route registration ────────────────────────────────────

	// POST /api/chat
	mux.HandleFunc("/api/chat", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method tidak diizinkan")
			return
		}
		h.PostChat(w, r)
	})

	// GET /api/chat/{session_id}/history  &  DELETE /api/chat/{session_id}
	mux.HandleFunc("/api/chat/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/history"):
			h.GetHistory(w, r)
		case r.Method == http.MethodDelete:
			h.DeleteSession(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method tidak diizinkan")
		}
	})

	// GET /api/health
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method tidak diizinkan")
			return
		}
		h.GetHealth(w, r)
	})

	// ── Terapkan middleware (CORS → logging → recovery) ───────
	return recovery(logging(cors(mux, allowedOrigins)))
}

// ─────────────────────────────────────────────
// MIDDLEWARE
// ─────────────────────────────────────────────

// cors menambahkan header CORS ke setiap respons.
// allowedOrigins: "*" untuk semua, atau "http://localhost:5173" untuk spesifik.
func cors(next http.Handler, allowedOrigins string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", allowedOrigins)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// Preflight request
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		} 
		next.ServeHTTP(w, r)
	})
}

// logging mencatat setiap request: method, path, status, durasi.
func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		log.Printf("[%s] %s %s → %d (%s)",
			r.Method, r.URL.Path, r.RemoteAddr,
			rw.status, time.Since(start),
		)
	})
}

// recovery menangkap panic agar server tidak crash.
func recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("[PANIC] %v\n%s", err, debug.Stack())
				writeError(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// responseWriter membungkus http.ResponseWriter agar status code bisa dibaca.
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(status int) {
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
}