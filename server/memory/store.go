package memory

import (
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

// SQLiteStore implements MemoryStore backed by SQLite.
type SQLiteStore struct {
	db *sql.DB
	mu sync.RWMutex

	stmtSave      *sql.Stmt
	stmtGet       *sql.Stmt
	stmtDelete    *sql.Stmt
	stmtIncAccess *sql.Stmt
	stmtCount     *sql.Stmt
}

// NewSQLiteStore opens (or creates) a SQLite database at dbPath and
// initialises the schema.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("memory store: mkdir: %w", err)
	}

	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=1")
	if err != nil {
		return nil, fmt.Errorf("memory store: open: %w", err)
	}

	db.SetMaxOpenConns(1) // SQLite is single-writer
	db.SetMaxIdleConns(2)

	if err := InitSchema(db); err != nil {
		db.Close()
		return nil, err
	}

	s := &SQLiteStore{db: db}
	if err := s.prepareStatements(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *SQLiteStore) prepareStatements() error {
	var err error

	s.stmtSave, err = s.db.Prepare(`
		INSERT OR REPLACE INTO memories
			(id, user_id, type, content, summary, importance, embedding,
			 access_count, pinned, archived, created_at, updated_at, last_accessed)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare save: %w", err)
	}

	s.stmtGet, err = s.db.Prepare(`
		SELECT id, user_id, type, content, summary, importance, embedding,
		       access_count, pinned, archived, created_at, updated_at, last_accessed
		FROM memories WHERE id = ?`)
	if err != nil {
		return fmt.Errorf("prepare get: %w", err)
	}

	s.stmtDelete, err = s.db.Prepare(`DELETE FROM memories WHERE id = ?`)
	if err != nil {
		return fmt.Errorf("prepare delete: %w", err)
	}

	s.stmtIncAccess, err = s.db.Prepare(`
		UPDATE memories
		SET access_count = access_count + 1,
		    last_accessed = ?
		WHERE id = ?`)
	if err != nil {
		return fmt.Errorf("prepare inc_access: %w", err)
	}

	s.stmtCount, err = s.db.Prepare(`SELECT COUNT(*) FROM memories WHERE user_id = ?`)
	if err != nil {
		return fmt.Errorf("prepare count: %w", err)
	}

	return nil
}

// Save persists a memory. If ID is empty, a new UUID is assigned.
func (s *SQLiteStore) Save(ctx context.Context, mem *Memory) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if mem.ID == "" {
		mem.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	if mem.CreatedAt.IsZero() {
		mem.CreatedAt = now
	}
	mem.UpdatedAt = now
	if mem.LastAccessed.IsZero() {
		mem.LastAccessed = now
	}

	embBlob := encodeEmbedding(mem.Embedding)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("memory save: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	_, err = tx.StmtContext(ctx, s.stmtSave).ExecContext(ctx,
		mem.ID, mem.UserID, string(mem.Type), mem.Content, mem.Summary,
		mem.Importance, embBlob, mem.AccessCount,
		boolToInt(mem.Pinned), boolToInt(mem.Archived),
		mem.CreatedAt.Format(time.RFC3339Nano),
		mem.UpdatedAt.Format(time.RFC3339Nano),
		mem.LastAccessed.Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("memory save: insert: %w", err)
	}

	// Handle tags
	if len(mem.Tags) > 0 {
		// Clear existing tags
		if _, err := tx.ExecContext(ctx, `DELETE FROM memory_tags WHERE memory_id = ?`, mem.ID); err != nil {
			return fmt.Errorf("memory save: clear tags: %w", err)
		}
		for _, tag := range mem.Tags {
			tag = strings.TrimSpace(strings.ToLower(tag))
			if tag == "" {
				continue
			}
			// Upsert tag
			if _, err := tx.ExecContext(ctx,
				`INSERT OR IGNORE INTO tags (name) VALUES (?)`, tag); err != nil {
				return fmt.Errorf("memory save: upsert tag: %w", err)
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT OR IGNORE INTO memory_tags (memory_id, tag_id)
				 SELECT ?, id FROM tags WHERE name = ?`, mem.ID, tag); err != nil {
				return fmt.Errorf("memory save: link tag: %w", err)
			}
		}
	}

	return tx.Commit()
}

// Get retrieves a single memory by ID.
func (s *SQLiteStore) Get(ctx context.Context, id string) (*Memory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	mem, err := s.scanMemory(s.stmtGet.QueryRowContext(ctx, id))
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("memory not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("memory get: %w", err)
	}

	// Load tags
	mem.Tags, _ = s.loadTags(ctx, id)
	return mem, nil
}

// List returns memories matching the given options.
func (s *SQLiteStore) List(ctx context.Context, userID string, opts ListOptions) ([]*Memory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var where []string
	var args []any

	where = append(where, "user_id = ?")
	args = append(args, userID)

	if opts.Type != "" {
		where = append(where, "type = ?")
		args = append(args, string(opts.Type))
	}
	if opts.Pinned != nil {
		where = append(where, "pinned = ?")
		args = append(args, boolToInt(*opts.Pinned))
	}
	if opts.Archived != nil {
		where = append(where, "archived = ?")
		args = append(args, boolToInt(*opts.Archived))
	}

	query := "SELECT id, user_id, type, content, summary, importance, embedding, " +
		"access_count, pinned, archived, created_at, updated_at, last_accessed " +
		"FROM memories WHERE " + strings.Join(where, " AND ") +
		" ORDER BY importance DESC, created_at DESC"

	if opts.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", opts.Limit)
	}
	if opts.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", opts.Offset)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("memory list: %w", err)
	}
	defer rows.Close()

	var memories []*Memory
	for rows.Next() {
		mem, err := s.scanMemoryRow(rows)
		if err != nil {
			return nil, fmt.Errorf("memory list scan: %w", err)
		}
		memories = append(memories, mem)
	}
	return memories, rows.Err()
}

// Update modifies an existing memory.
func (s *SQLiteStore) Update(ctx context.Context, mem *Memory) error {
	mem.UpdatedAt = time.Now().UTC()
	// Re-use Save which does INSERT OR REPLACE
	return s.Save(ctx, mem)
}

// Delete removes a memory by ID.
func (s *SQLiteStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.stmtDelete.ExecContext(ctx, id)
	if err != nil {
		return fmt.Errorf("memory delete: %w", err)
	}
	return nil
}

// IncrementAccess bumps the access counter and last_accessed timestamp.
func (s *SQLiteStore) IncrementAccess(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.stmtIncAccess.ExecContext(ctx,
		time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return fmt.Errorf("memory inc_access: %w", err)
	}
	return nil
}

// GetByIDs retrieves multiple memories in a single query.
func (s *SQLiteStore) GetByIDs(ctx context.Context, ids []string) ([]*Memory, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := "SELECT id, user_id, type, content, summary, importance, embedding, " +
		"access_count, pinned, archived, created_at, updated_at, last_accessed " +
		"FROM memories WHERE id IN (" + strings.Join(placeholders, ",") + ")"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("memory get_by_ids: %w", err)
	}
	defer rows.Close()

	var memories []*Memory
	for rows.Next() {
		mem, err := s.scanMemoryRow(rows)
		if err != nil {
			return nil, fmt.Errorf("memory get_by_ids scan: %w", err)
		}
		memories = append(memories, mem)
	}
	return memories, rows.Err()
}

// CountByUser returns the total number of memories for a user.
func (s *SQLiteStore) CountByUser(ctx context.Context, userID string) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var count int64
	err := s.stmtCount.QueryRowContext(ctx, userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("memory count: %w", err)
	}
	return count, nil
}

// ArchiveOlderThan marks stale, low-importance memories as archived.
func (s *SQLiteStore) ArchiveOlderThan(ctx context.Context, before time.Time, minImportance float64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.ExecContext(ctx,
		`UPDATE memories SET archived = 1, updated_at = ?
		 WHERE archived = 0 AND pinned = 0
		   AND last_accessed < ? AND importance < ?`,
		time.Now().UTC().Format(time.RFC3339Nano),
		before.Format(time.RFC3339Nano),
		minImportance,
	)
	if err != nil {
		return 0, fmt.Errorf("memory archive: %w", err)
	}
	return result.RowsAffected()
}

// Close releases all resources.
func (s *SQLiteStore) Close() error {
	s.stmtSave.Close()
	s.stmtGet.Close()
	s.stmtDelete.Close()
	s.stmtIncAccess.Close()
	s.stmtCount.Close()
	return s.db.Close()
}

// -- internal helpers --

type rowScanner interface {
	Scan(dest ...any) error
}

func (s *SQLiteStore) scanMemory(row rowScanner) (*Memory, error) {
	var mem Memory
	var embBlob []byte
	var typ, createdAt, updatedAt, lastAccessed string
	var pinned, archived int

	err := row.Scan(
		&mem.ID, &mem.UserID, &typ, &mem.Content, &mem.Summary,
		&mem.Importance, &embBlob, &mem.AccessCount,
		&pinned, &archived, &createdAt, &updatedAt, &lastAccessed,
	)
	if err != nil {
		return nil, err
	}

	mem.Type = MemoryType(typ)
	mem.Pinned = pinned != 0
	mem.Archived = archived != 0
	mem.Embedding = decodeEmbedding(embBlob)
	mem.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	mem.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	mem.LastAccessed, _ = time.Parse(time.RFC3339Nano, lastAccessed)

	return &mem, nil
}

func (s *SQLiteStore) scanMemoryRow(rows *sql.Rows) (*Memory, error) {
	return s.scanMemory(rows)
}

func (s *SQLiteStore) loadTags(ctx context.Context, memID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT t.name FROM tags t
		 JOIN memory_tags mt ON mt.tag_id = t.id
		 WHERE mt.memory_id = ?`, memID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return tags, err
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}

// encodeEmbedding serialises a float32 slice as a little-endian binary blob.
func encodeEmbedding(emb []float32) []byte {
	if len(emb) == 0 {
		return nil
	}
	buf := make([]byte, len(emb)*4)
	for i, v := range emb {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return buf
}

// decodeEmbedding deserialises a little-endian binary blob into float32 slice.
func decodeEmbedding(buf []byte) []float32 {
	if len(buf) == 0 {
		return nil
	}
	n := len(buf) / 4
	emb := make([]float32, n)
	for i := 0; i < n; i++ {
		emb[i] = math.Float32frombits(binary.LittleEndian.Uint32(buf[i*4:]))
	}
	return emb
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
