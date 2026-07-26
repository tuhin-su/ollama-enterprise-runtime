package memory

import (
	"database/sql"
	"fmt"
)

const schemaSQL = `
PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;
PRAGMA foreign_keys = ON;
PRAGMA synchronous = NORMAL;

CREATE TABLE IF NOT EXISTS memories (
	id            TEXT PRIMARY KEY,
	user_id       TEXT NOT NULL,
	type          TEXT NOT NULL DEFAULT 'semantic',
	content       TEXT NOT NULL,
	summary       TEXT DEFAULT '',
	importance    REAL DEFAULT 0.5,
	embedding     BLOB,
	access_count  INTEGER DEFAULT 0,
	pinned        INTEGER DEFAULT 0,
	archived      INTEGER DEFAULT 0,
	created_at    TEXT NOT NULL,
	updated_at    TEXT NOT NULL,
	last_accessed TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_memories_user_id ON memories(user_id);
CREATE INDEX IF NOT EXISTS idx_memories_type ON memories(user_id, type);
CREATE INDEX IF NOT EXISTS idx_memories_importance ON memories(importance DESC);
CREATE INDEX IF NOT EXISTS idx_memories_created ON memories(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_memories_archived ON memories(archived, user_id);
CREATE INDEX IF NOT EXISTS idx_memories_pinned ON memories(pinned, user_id);
CREATE INDEX IF NOT EXISTS idx_memories_last_accessed ON memories(last_accessed);

CREATE TABLE IF NOT EXISTS tags (
	id   INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT UNIQUE NOT NULL
);

CREATE TABLE IF NOT EXISTS memory_tags (
	memory_id TEXT NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
	tag_id    INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
	PRIMARY KEY (memory_id, tag_id)
);

CREATE INDEX IF NOT EXISTS idx_memory_tags_tag ON memory_tags(tag_id);

CREATE TABLE IF NOT EXISTS conversations (
	id         TEXT PRIMARY KEY,
	user_id    TEXT NOT NULL,
	model      TEXT DEFAULT '',
	started_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_conversations_user ON conversations(user_id);

CREATE TABLE IF NOT EXISTS summaries (
	id              TEXT PRIMARY KEY,
	conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
	user_id         TEXT NOT NULL,
	content         TEXT NOT NULL,
	created_at      TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_summaries_conv ON summaries(conversation_id);
CREATE INDEX IF NOT EXISTS idx_summaries_user ON summaries(user_id);
`

// InitSchema creates all tables and indexes. It is idempotent.
func InitSchema(db *sql.DB) error {
	_, err := db.Exec(schemaSQL)
	if err != nil {
		return fmt.Errorf("memory: init schema: %w", err)
	}
	return nil
}
