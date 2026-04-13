// internal/memory/store.go
// Menyimpan dan memuat riwayat percakapan dari MySQL.

package memory

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// Message merepresentasikan satu baris riwayat percakapan.
type Message struct {
	Role      string
	Content   string
	CreatedAt time.Time
}

// Store mengelola koneksi dan operasi database.
type Store struct {
	db *sql.DB
}

// New membuat koneksi ke MySQL dan menginisialisasi tabel.
func New(dsn string) (*Store, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("gagal membuka koneksi: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("gagal ping database: %w", err)
	}

	s := &Store{db: db}
	if err := s.initTable(); err != nil {
		return nil, fmt.Errorf("gagal inisialisasi tabel: %w", err)
	}

	return s, nil
}

// initTable membuat tabel chat_history jika belum ada.
func (s *Store) initTable() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS chat_history (
			id         INT AUTO_INCREMENT PRIMARY KEY,
			session_id VARCHAR(100),
			role       VARCHAR(20),
			message    TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	return err
}

// Save menyimpan satu pesan ke database.
func (s *Store) Save(sessionID, role, message string) error {
	_, err := s.db.Exec(
		"INSERT INTO chat_history (session_id, role, message) VALUES (?, ?, ?)",
		sessionID, role, message,
	)
	return err
}

// LoadHistory memuat riwayat percakapan terakhir dalam urutan kronologis.
func (s *Store) LoadHistory(sessionID string, limit int) ([]Message, error) {
	rows, err := s.db.Query(`
		SELECT role, message, created_at
		FROM (
			SELECT role, message, created_at
			FROM chat_history
			WHERE session_id = ?
			ORDER BY created_at DESC
			LIMIT ?
		) sub
		ORDER BY created_at ASC
	`, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.Role, &m.Content, &m.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

// Close menutup koneksi database.
func (s *Store) Close() error {
	return s.db.Close()
}