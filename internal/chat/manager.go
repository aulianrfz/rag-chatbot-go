// internal/chat/manager.go
// SessionManager mengelola banyak Session sekaligus (multi-user).
// Setiap session_id unik mendapat Session-nya sendiri.

package chat

import (
	"sync"

	"rag-chatbot-go/internal/llm"
	"rag-chatbot-go/internal/memory"
	"rag-chatbot-go/internal/rag"
)

// Manager membuat dan menyimpan Session berdasarkan session_id.
type Manager struct {
	mu        sync.RWMutex
	sessions  map[string]*Session
	ragEngine *rag.Engine
	provider  llm.LLMProvider
	store     *memory.Store // nil jika DB tidak tersambung
}

// NewManager membuat Manager baru.
func NewManager(ragEngine *rag.Engine, provider llm.LLMProvider, store *memory.Store) *Manager {
	return &Manager{
		sessions:  make(map[string]*Session),
		ragEngine: ragEngine,
		provider:  provider,
		store:     store,
	}
}

// GetOrCreate mengembalikan Session yang ada atau membuat yang baru.
func (m *Manager) GetOrCreate(sessionID string) *Session {
	// Fast path: baca dengan RLock
	m.mu.RLock()
	s, ok := m.sessions[sessionID]
	m.mu.RUnlock()
	if ok {
		return s
	}

	// Slow path: buat session baru
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check setelah acquire write lock
	if s, ok = m.sessions[sessionID]; ok {
		return s
	}

	s = NewSession(sessionID, m.ragEngine, m.provider, m.store)
	m.sessions[sessionID] = s
	return s
}

// Delete menghapus session dari memory.
func (m *Manager) Delete(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, sessionID)
}

// ActiveSessions mengembalikan jumlah session aktif saat ini.
func (m *Manager) ActiveSessions() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}