// internal/api/handler.go
// HTTP handlers untuk semua endpoint chatbot API.
//
// Endpoint:
//   POST   /api/chat                  → kirim pesan, dapat balasan
//   GET    /api/chat/:session_id/history → riwayat percakapan
//   DELETE /api/chat/:session_id       → hapus/reset session
//   GET    /api/health                 → status server

package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"rag-chatbot-go/internal/chat"
	"rag-chatbot-go/internal/rag"
)

// Handler menyimpan dependency yang dibutuhkan oleh semua handler.
type Handler struct {
	manager   *chat.Manager
	ragEngine *rag.Engine
	startTime time.Time
}

// NewHandler membuat Handler baru.
func NewHandler(manager *chat.Manager, ragEngine *rag.Engine) *Handler {
	return &Handler{
		manager:   manager,
		ragEngine: ragEngine,
		startTime: time.Now(),
	}
}

// ─────────────────────────────────────────────
// REQUEST / RESPONSE TYPES
// ─────────────────────────────────────────────

type ChatRequest struct {
	SessionID string `json:"session_id"` // wajib
	Message   string `json:"message"`    // wajib
}

type ChatResponse struct {
	SessionID string `json:"session_id"`
	Message   string `json:"message"`
	Reply     string `json:"reply"`
}

type HistoryResponse struct {
	SessionID string              `json:"session_id"`
	History   []chat.HistoryEntry `json:"history"`
}

type HealthResponse struct {
	Status         string `json:"status"`
	Provider       string `json:"provider"`
	ActiveSessions int    `json:"active_sessions"`
	UptimeSeconds  int64  `json:"uptime_seconds"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

// ─────────────────────────────────────────────
// HANDLERS
// ─────────────────────────────────────────────

// PostChat menangani POST /api/chat
// Body: { "session_id": "...", "message": "..." }
func (h *Handler) PostChat(w http.ResponseWriter, r *http.Request) {
	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "body JSON tidak valid")
		return
	}

	req.SessionID = strings.TrimSpace(req.SessionID)
	req.Message = strings.TrimSpace(req.Message)

	if req.SessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id wajib diisi")
		return
	}
	if req.Message == "" {
		writeError(w, http.StatusBadRequest, "message wajib diisi")
		return
	}

	session := h.manager.GetOrCreate(req.SessionID)
	reply, err := session.Chat(r.Context(), req.Message)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "gagal generate respons: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, ChatResponse{
		SessionID: req.SessionID,
		Message:   req.Message,
		Reply:     reply,
	})
}

// GetHistory menangani GET /api/chat/{session_id}/history
func (h *Handler) GetHistory(w http.ResponseWriter, r *http.Request) {
	sessionID := extractSegment(r.URL.Path, "/api/chat/", "/history")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id tidak ditemukan di URL")
		return
	}

	session := h.manager.GetOrCreate(sessionID)
	writeJSON(w, http.StatusOK, HistoryResponse{
		SessionID: sessionID,
		History:   session.History(),
	})
}

// DeleteSession menangani DELETE /api/chat/{session_id}
func (h *Handler) DeleteSession(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimPrefix(r.URL.Path, "/api/chat/")
	sessionID = strings.TrimSpace(sessionID)

	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id tidak ditemukan di URL")
		return
	}

	h.manager.Delete(sessionID)
	writeJSON(w, http.StatusOK, map[string]string{
		"message":    "session berhasil dihapus",
		"session_id": sessionID,
	})
}

// GetHealth menangani GET /api/health
func (h *Handler) GetHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, HealthResponse{
		Status:         "ok",
		ActiveSessions: h.manager.ActiveSessions(),
		UptimeSeconds:  int64(time.Since(h.startTime).Seconds()),
	})
}

// ─────────────────────────────────────────────
// HELPERS
// ─────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, ErrorResponse{Error: msg})
}

// extractSegment mengambil segmen URL di antara prefix dan suffix.
// Contoh: "/api/chat/abc123/history" → prefix="/api/chat/", suffix="/history" → "abc123"
func extractSegment(path, prefix, suffix string) string {
	s := strings.TrimPrefix(path, prefix)
	s = strings.TrimSuffix(s, suffix)
	return strings.TrimSpace(s)
}