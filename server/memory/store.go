package memory

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
	"sort"

	"github.com/google/uuid"
	"github.com/apache/arrow/go/v17/arrow"
	"github.com/apache/arrow/go/v17/arrow/array"
	"github.com/apache/arrow/go/v17/arrow/memory"

	"github.com/lancedb/lancedb-go/pkg/contracts"
	"github.com/lancedb/lancedb-go/pkg/lancedb"
)

// LanceDBStore implements MemoryStore backed by LanceDB.
type LanceDBStore struct {
	conn  contracts.IConnection
	mu    sync.RWMutex
	dbDir string
	dim   int
}

// NewLanceDBStore opens (or creates) a LanceDB database at dbPath.
func NewLanceDBStore(dbPath string) (*LanceDBStore, error) {
	dbDir := dbPath
	if strings.HasSuffix(dbPath, ".db") {
		dbDir = dbPath[:len(dbPath)-3] + ".lance"
	}

	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		return nil, fmt.Errorf("memory store: mkdir: %w", err)
	}

	conn, err := lancedb.Connect(context.Background(), dbDir, nil)
	if err != nil {
		return nil, fmt.Errorf("memory store: connect: %w", err)
	}

	return &LanceDBStore{
		conn:  conn,
		dbDir: dbDir,
		dim:   768, // default dimension (e.g. nomic-embed-text)
	}, nil
}

func getArrowSchema(dim int) *arrow.Schema {
	return arrow.NewSchema([]arrow.Field{
		{Name: "id", Type: arrow.BinaryTypes.String, Nullable: false},
		{Name: "user_id", Type: arrow.BinaryTypes.String, Nullable: false},
		{Name: "type", Type: arrow.BinaryTypes.String, Nullable: false},
		{Name: "content", Type: arrow.BinaryTypes.String, Nullable: false},
		{Name: "summary", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "importance", Type: arrow.PrimitiveTypes.Float64, Nullable: false},
		{Name: "embedding", Type: arrow.FixedSizeListOf(int32(dim), arrow.PrimitiveTypes.Float32), Nullable: false},
		{Name: "tags", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "access_count", Type: arrow.PrimitiveTypes.Int64, Nullable: false},
		{Name: "pinned", Type: arrow.FixedWidthTypes.Boolean, Nullable: false},
		{Name: "archived", Type: arrow.FixedWidthTypes.Boolean, Nullable: false},
		{Name: "created_at", Type: arrow.BinaryTypes.String, Nullable: false},
		{Name: "updated_at", Type: arrow.BinaryTypes.String, Nullable: false},
		{Name: "last_accessed", Type: arrow.BinaryTypes.String, Nullable: false},
	}, nil)
}

func memoriesToRecord(memories []*Memory, dim int) (arrow.Record, error) {
	pool := memory.NewGoAllocator()
	arrowSchema := getArrowSchema(dim)

	idBuilder := array.NewStringBuilder(pool)
	defer idBuilder.Release()
	userIDBuilder := array.NewStringBuilder(pool)
	defer userIDBuilder.Release()
	typeBuilder := array.NewStringBuilder(pool)
	defer typeBuilder.Release()
	contentBuilder := array.NewStringBuilder(pool)
	defer contentBuilder.Release()
	summaryBuilder := array.NewStringBuilder(pool)
	defer summaryBuilder.Release()
	importanceBuilder := array.NewFloat64Builder(pool)
	defer importanceBuilder.Release()
	tagsBuilder := array.NewStringBuilder(pool)
	defer tagsBuilder.Release()
	accessCountBuilder := array.NewInt64Builder(pool)
	defer accessCountBuilder.Release()
	pinnedBuilder := array.NewBooleanBuilder(pool)
	defer pinnedBuilder.Release()
	archivedBuilder := array.NewBooleanBuilder(pool)
	defer archivedBuilder.Release()
	createdAtBuilder := array.NewStringBuilder(pool)
	defer createdAtBuilder.Release()
	updatedAtBuilder := array.NewStringBuilder(pool)
	defer updatedAtBuilder.Release()
	lastAccessedBuilder := array.NewStringBuilder(pool)
	defer lastAccessedBuilder.Release()

	var flatEmbeddings []float32
	for _, mem := range memories {
		idBuilder.Append(mem.ID)
		userIDBuilder.Append(mem.UserID)
		typeBuilder.Append(string(mem.Type))
		contentBuilder.Append(mem.Content)
		summaryBuilder.Append(mem.Summary)
		importanceBuilder.Append(mem.Importance)

		emb := mem.Embedding
		if len(emb) != dim {
			newEmb := make([]float32, dim)
			copy(newEmb, emb)
			emb = newEmb
		}
		flatEmbeddings = append(flatEmbeddings, emb...)

		tagsBuilder.Append(strings.Join(mem.Tags, ","))
		accessCountBuilder.Append(mem.AccessCount)
		pinnedBuilder.Append(mem.Pinned)
		archivedBuilder.Append(mem.Archived)
		createdAtBuilder.Append(mem.CreatedAt.Format(time.RFC3339Nano))
		updatedAtBuilder.Append(mem.UpdatedAt.Format(time.RFC3339Nano))
		lastAccessedBuilder.Append(mem.LastAccessed.Format(time.RFC3339Nano))
	}

	embFloatBuilder := array.NewFloat32Builder(pool)
	defer embFloatBuilder.Release()
	embFloatBuilder.AppendValues(flatEmbeddings, nil)
	embFloatArray := embFloatBuilder.NewArray()
	defer embFloatArray.Release()

	embeddingListType := arrow.FixedSizeListOf(int32(dim), arrow.PrimitiveTypes.Float32)
	embeddingArray := array.NewFixedSizeListData(
		array.NewData(embeddingListType, len(memories), []*memory.Buffer{nil}, []arrow.ArrayData{embFloatArray.Data()}, 0, 0),
	)
	defer embeddingArray.Release()

	idArr := idBuilder.NewArray()
	defer idArr.Release()
	userArr := userIDBuilder.NewArray()
	defer userArr.Release()
	typeArr := typeBuilder.NewArray()
	defer typeArr.Release()
	contentArr := contentBuilder.NewArray()
	defer contentArr.Release()
	summaryArr := summaryBuilder.NewArray()
	defer summaryArr.Release()
	importanceArr := importanceBuilder.NewArray()
	defer importanceArr.Release()
	tagsArr := tagsBuilder.NewArray()
	defer tagsArr.Release()
	accessArr := accessCountBuilder.NewArray()
	defer accessArr.Release()
	pinnedArr := pinnedBuilder.NewArray()
	defer pinnedArr.Release()
	archivedArr := archivedBuilder.NewArray()
	defer archivedArr.Release()
	createdArr := createdAtBuilder.NewArray()
	defer createdArr.Release()
	updatedArr := updatedAtBuilder.NewArray()
	defer updatedArr.Release()
	accessedArr := lastAccessedBuilder.NewArray()
	defer accessedArr.Release()

	columns := []arrow.Array{
		idArr, userArr, typeArr, contentArr, summaryArr, importanceArr,
		embeddingArray, tagsArr, accessArr, pinnedArr, archivedArr,
		createdArr, updatedArr, accessedArr,
	}

	return array.NewRecord(arrowSchema, columns, int64(len(memories))), nil
}

func mapToMemory(row map[string]interface{}) *Memory {
	var mem Memory
	if val, ok := row["id"].(string); ok {
		mem.ID = val
	}
	if val, ok := row["user_id"].(string); ok {
		mem.UserID = val
	}
	if val, ok := row["type"].(string); ok {
		mem.Type = MemoryType(val)
	}
	if val, ok := row["content"].(string); ok {
		mem.Content = val
	}
	if val, ok := row["summary"].(string); ok {
		mem.Summary = val
	}
	if val, ok := row["importance"].(float64); ok {
		mem.Importance = val
	}
	if val, ok := row["tags"].(string); ok {
		if val != "" {
			mem.Tags = strings.Split(val, ",")
		}
	}
	if val, ok := row["access_count"].(int64); ok {
		mem.AccessCount = val
	} else if val, ok := row["access_count"].(int32); ok {
		mem.AccessCount = int64(val)
	} else if val, ok := row["access_count"].(float64); ok {
		mem.AccessCount = int64(val)
	}
	if val, ok := row["pinned"].(bool); ok {
		mem.Pinned = val
	}
	if val, ok := row["archived"].(bool); ok {
		mem.Archived = val
	}
	if val, ok := row["created_at"].(string); ok {
		mem.CreatedAt, _ = time.Parse(time.RFC3339Nano, val)
	}
	if val, ok := row["updated_at"].(string); ok {
		mem.UpdatedAt, _ = time.Parse(time.RFC3339Nano, val)
	}
	if val, ok := row["last_accessed"].(string); ok {
		mem.LastAccessed, _ = time.Parse(time.RFC3339Nano, val)
	}

	if val, ok := row["embedding"].([]float32); ok {
		mem.Embedding = val
	} else if val, ok := row["embedding"].([]interface{}); ok {
		emb := make([]float32, len(val))
		for i, v := range val {
			if f, ok := v.(float32); ok {
				emb[i] = f
			} else if f, ok := v.(float64); ok {
				emb[i] = float32(f)
			}
		}
		mem.Embedding = emb
	}

	return &mem
}

func (s *LanceDBStore) getTable(ctx context.Context, dim int) (contracts.ITable, error) {
	if dim <= 0 {
		dim = s.dim
	}
	s.dim = dim

	table, err := s.conn.OpenTable(ctx, "memories")
	if err == nil {
		return table, nil
	}

	arrowSchema := getArrowSchema(dim)
	schema, err := lancedb.NewSchema(arrowSchema)
	if err != nil {
		return nil, fmt.Errorf("create schema: %w", err)
	}

	table, err = s.conn.CreateTable(ctx, "memories", schema)
	if err != nil {
		return nil, fmt.Errorf("create table: %w", err)
	}
	return table, nil
}

// Save persists a memory.
func (s *LanceDBStore) Save(ctx context.Context, mem *Memory) error {
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

	dim := len(mem.Embedding)
	if dim == 0 {
		dim = s.dim
	}

	table, err := s.getTable(ctx, dim)
	if err != nil {
		return fmt.Errorf("lancedb save: %w", err)
	}
	defer table.Close()

	escapedID := strings.ReplaceAll(mem.ID, "'", "''")
	_ = table.Delete(ctx, fmt.Sprintf("id = '%s'", escapedID))

	record, err := memoriesToRecord([]*Memory{mem}, dim)
	if err != nil {
		return fmt.Errorf("lancedb save record build: %w", err)
	}
	defer record.Release()

	err = table.Add(ctx, record, nil)
	if err != nil {
		return fmt.Errorf("lancedb save add: %w", err)
	}

	return nil
}

// Get retrieves a single memory by ID.
func (s *LanceDBStore) Get(ctx context.Context, id string) (*Memory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	table, err := s.getTable(ctx, s.dim)
	if err != nil {
		return nil, fmt.Errorf("lancedb get: %w", err)
	}
	defer table.Close()

	escapedID := strings.ReplaceAll(id, "'", "''")
	rows, err := table.SelectWithFilter(ctx, fmt.Sprintf("id = '%s'", escapedID))
	if err != nil {
		return nil, fmt.Errorf("lancedb get select: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("memory not found: %s", id)
	}

	return mapToMemory(rows[0]), nil
}

// List returns memories matching the given options.
func (s *LanceDBStore) List(ctx context.Context, userID string, opts ListOptions) ([]*Memory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	table, err := s.getTable(ctx, s.dim)
	if err != nil {
		return nil, fmt.Errorf("lancedb list: %w", err)
	}
	defer table.Close()

	var filters []string
	escapedUserID := strings.ReplaceAll(userID, "'", "''")
	filters = append(filters, fmt.Sprintf("user_id = '%s'", escapedUserID))

	if opts.Type != "" {
		filters = append(filters, fmt.Sprintf("type = '%s'", string(opts.Type)))
	}
	if opts.Pinned != nil {
		if *opts.Pinned {
			filters = append(filters, "pinned = true")
		} else {
			filters = append(filters, "pinned = false")
		}
	}
	if opts.Archived != nil {
		if *opts.Archived {
			filters = append(filters, "archived = true")
		} else {
			filters = append(filters, "archived = false")
		}
	}

	filterStr := strings.Join(filters, " AND ")

	var rows []map[string]interface{}
	if opts.Limit > 0 || opts.Offset > 0 {
		limitVal := opts.Limit
		offsetVal := opts.Offset
		qConfig := contracts.QueryConfig{
			Where: filterStr,
		}
		if limitVal > 0 {
			qConfig.Limit = &limitVal
		}
		if offsetVal > 0 {
			qConfig.Offset = &offsetVal
		}
		rows, err = table.Select(ctx, qConfig)
	} else {
		rows, err = table.SelectWithFilter(ctx, filterStr)
	}

	if err != nil {
		return nil, fmt.Errorf("lancedb list select: %w", err)
	}

	memories := make([]*Memory, len(rows))
	for i, row := range rows {
		memories[i] = mapToMemory(row)
	}

	sort.Slice(memories, func(i, j int) bool {
		if memories[i].Importance != memories[j].Importance {
			return memories[i].Importance > memories[j].Importance
		}
		return memories[i].CreatedAt.After(memories[j].CreatedAt)
	})

	return memories, nil
}

// Update modifies an existing memory.
func (s *LanceDBStore) Update(ctx context.Context, mem *Memory) error {
	mem.UpdatedAt = time.Now().UTC()
	return s.Save(ctx, mem)
}

// Delete removes a memory by ID.
func (s *LanceDBStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	table, err := s.getTable(ctx, s.dim)
	if err != nil {
		return fmt.Errorf("lancedb delete: %w", err)
	}
	defer table.Close()

	escapedID := strings.ReplaceAll(id, "'", "''")
	err = table.Delete(ctx, fmt.Sprintf("id = '%s'", escapedID))
	if err != nil {
		return fmt.Errorf("lancedb delete: %w", err)
	}
	return nil
}

// IncrementAccess bumps the access counter and last_accessed timestamp.
func (s *LanceDBStore) IncrementAccess(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	table, err := s.getTable(ctx, s.dim)
	if err != nil {
		return fmt.Errorf("lancedb inc_access: %w", err)
	}
	defer table.Close()

	escapedID := strings.ReplaceAll(id, "'", "''")
	rows, err := table.SelectWithFilter(ctx, fmt.Sprintf("id = '%s'", escapedID))
	if err != nil || len(rows) == 0 {
		return fmt.Errorf("lancedb inc_access not found: %w", err)
	}

	mem := mapToMemory(rows[0])
	mem.AccessCount++
	mem.LastAccessed = time.Now().UTC()

	_ = table.Delete(ctx, fmt.Sprintf("id = '%s'", escapedID))

	record, err := memoriesToRecord([]*Memory{mem}, s.dim)
	if err != nil {
		return err
	}
	defer record.Release()

	return table.Add(ctx, record, nil)
}

// GetByIDs retrieves multiple memories in a single query.
func (s *LanceDBStore) GetByIDs(ctx context.Context, ids []string) ([]*Memory, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	table, err := s.getTable(ctx, s.dim)
	if err != nil {
		return nil, fmt.Errorf("lancedb get_by_ids: %w", err)
	}
	defer table.Close()

	var escapedIDs []string
	for _, id := range ids {
		escapedIDs = append(escapedIDs, fmt.Sprintf("'%s'", strings.ReplaceAll(id, "'", "''")))
	}

	filter := fmt.Sprintf("id IN (%s)", strings.Join(escapedIDs, ","))
	rows, err := table.SelectWithFilter(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("lancedb get_by_ids select: %w", err)
	}

	memories := make([]*Memory, len(rows))
	for i, row := range rows {
		memories[i] = mapToMemory(row)
	}
	return memories, nil
}

// CountByUser returns the total number of memories for a user.
func (s *LanceDBStore) CountByUser(ctx context.Context, userID string) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	table, err := s.getTable(ctx, s.dim)
	if err != nil {
		return 0, fmt.Errorf("lancedb count: %w", err)
	}
	defer table.Close()

	escapedUserID := strings.ReplaceAll(userID, "'", "''")
	rows, err := table.SelectWithFilter(ctx, fmt.Sprintf("user_id = '%s'", escapedUserID))
	if err != nil {
		return 0, fmt.Errorf("lancedb count select: %w", err)
	}
	return int64(len(rows)), nil
}

// ArchiveOlderThan marks stale, low-importance memories as archived.
func (s *LanceDBStore) ArchiveOlderThan(ctx context.Context, before time.Time, minImportance float64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	table, err := s.getTable(ctx, s.dim)
	if err != nil {
		return 0, fmt.Errorf("lancedb archive: %w", err)
	}
	defer table.Close()

	beforeStr := before.Format(time.RFC3339Nano)
	filter := fmt.Sprintf("archived = false AND pinned = false AND last_accessed < '%s' AND importance < %f", beforeStr, minImportance)
	rows, err := table.SelectWithFilter(ctx, filter)
	if err != nil {
		return 0, fmt.Errorf("lancedb archive select: %w", err)
	}

	if len(rows) == 0 {
		return 0, nil
	}

	for _, row := range rows {
		mem := mapToMemory(row)
		mem.Archived = true
		mem.UpdatedAt = time.Now().UTC()

		escapedID := strings.ReplaceAll(mem.ID, "'", "''")
		_ = table.Delete(ctx, fmt.Sprintf("id = '%s'", escapedID))
		record, err := memoriesToRecord([]*Memory{mem}, s.dim)
		if err == nil {
			_ = table.Add(ctx, record, nil)
			record.Release()
		}
	}

	return int64(len(rows)), nil
}

// Close releases all resources.
func (s *LanceDBStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conn.Close()
}

func getConversationArrowSchema() *arrow.Schema {
	return arrow.NewSchema([]arrow.Field{
		{Name: "id", Type: arrow.BinaryTypes.String, Nullable: false},
		{Name: "user_id", Type: arrow.BinaryTypes.String, Nullable: false},
		{Name: "model", Type: arrow.BinaryTypes.String, Nullable: false},
		{Name: "user_message", Type: arrow.BinaryTypes.String, Nullable: false},
		{Name: "assistant_message", Type: arrow.BinaryTypes.String, Nullable: false},
		{Name: "thinking", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "timestamp", Type: arrow.BinaryTypes.String, Nullable: false},
	}, nil)
}

func getSpecialMemoryArrowSchema(dim int) *arrow.Schema {
	return arrow.NewSchema([]arrow.Field{
		{Name: "id", Type: arrow.BinaryTypes.String, Nullable: false},
		{Name: "user_id", Type: arrow.BinaryTypes.String, Nullable: false},
		{Name: "key", Type: arrow.BinaryTypes.String, Nullable: false},
		{Name: "value", Type: arrow.BinaryTypes.String, Nullable: false},
		{Name: "embedding", Type: arrow.FixedSizeListOf(int32(dim), arrow.PrimitiveTypes.Float32), Nullable: false},
		{Name: "created_at", Type: arrow.BinaryTypes.String, Nullable: false},
		{Name: "updated_at", Type: arrow.BinaryTypes.String, Nullable: false},
	}, nil)
}

func conversationsToRecord(conversations []*Conversation) (arrow.Record, error) {
	pool := memory.NewGoAllocator()
	arrowSchema := getConversationArrowSchema()

	idBuilder := array.NewStringBuilder(pool)
	defer idBuilder.Release()
	userIDBuilder := array.NewStringBuilder(pool)
	defer userIDBuilder.Release()
	modelBuilder := array.NewStringBuilder(pool)
	defer modelBuilder.Release()
	userMsgBuilder := array.NewStringBuilder(pool)
	defer userMsgBuilder.Release()
	assistantMsgBuilder := array.NewStringBuilder(pool)
	defer assistantMsgBuilder.Release()
	thinkingBuilder := array.NewStringBuilder(pool)
	defer thinkingBuilder.Release()
	timestampBuilder := array.NewStringBuilder(pool)
	defer timestampBuilder.Release()

	for _, conv := range conversations {
		idBuilder.Append(conv.ID)
		userIDBuilder.Append(conv.UserID)
		modelBuilder.Append(conv.Model)
		userMsgBuilder.Append(conv.UserMessage)
		assistantMsgBuilder.Append(conv.AssistantMessage)
		thinkingBuilder.Append(conv.Thinking)
		timestampBuilder.Append(conv.Timestamp.Format(time.RFC3339Nano))
	}

	idArr := idBuilder.NewArray()
	defer idArr.Release()
	userArr := userIDBuilder.NewArray()
	defer userArr.Release()
	modelArr := modelBuilder.NewArray()
	defer modelArr.Release()
	userMsgArr := userMsgBuilder.NewArray()
	defer userMsgArr.Release()
	assistantMsgArr := assistantMsgBuilder.NewArray()
	defer assistantMsgArr.Release()
	thinkingArr := thinkingBuilder.NewArray()
	defer thinkingArr.Release()
	timestampArr := timestampBuilder.NewArray()
	defer timestampArr.Release()

	columns := []arrow.Array{
		idArr, userArr, modelArr, userMsgArr, assistantMsgArr, thinkingArr, timestampArr,
	}

	return array.NewRecord(arrowSchema, columns, int64(len(conversations))), nil
}

func specialMemoriesToRecord(memories []*SpecialMemory, dim int) (arrow.Record, error) {
	pool := memory.NewGoAllocator()
	arrowSchema := getSpecialMemoryArrowSchema(dim)

	idBuilder := array.NewStringBuilder(pool)
	defer idBuilder.Release()
	userIDBuilder := array.NewStringBuilder(pool)
	defer userIDBuilder.Release()
	keyBuilder := array.NewStringBuilder(pool)
	defer keyBuilder.Release()
	valueBuilder := array.NewStringBuilder(pool)
	defer valueBuilder.Release()
	createdAtBuilder := array.NewStringBuilder(pool)
	defer createdAtBuilder.Release()
	updatedAtBuilder := array.NewStringBuilder(pool)
	defer updatedAtBuilder.Release()

	var flatEmbeddings []float32
	for _, mem := range memories {
		idBuilder.Append(mem.ID)
		userIDBuilder.Append(mem.UserID)
		keyBuilder.Append(mem.Key)
		valueBuilder.Append(mem.Value)

		emb := mem.Embedding
		if len(emb) != dim {
			newEmb := make([]float32, dim)
			copy(newEmb, emb)
			emb = newEmb
		}
		flatEmbeddings = append(flatEmbeddings, emb...)

		createdAtBuilder.Append(mem.CreatedAt.Format(time.RFC3339Nano))
		updatedAtBuilder.Append(mem.UpdatedAt.Format(time.RFC3339Nano))
	}

	embFloatBuilder := array.NewFloat32Builder(pool)
	defer embFloatBuilder.Release()
	embFloatBuilder.AppendValues(flatEmbeddings, nil)
	embFloatArray := embFloatBuilder.NewArray()
	defer embFloatArray.Release()

	embeddingListType := arrow.FixedSizeListOf(int32(dim), arrow.PrimitiveTypes.Float32)
	embeddingArray := array.NewFixedSizeListData(
		array.NewData(embeddingListType, len(memories), []*memory.Buffer{nil}, []arrow.ArrayData{embFloatArray.Data()}, 0, 0),
	)
	defer embeddingArray.Release()

	idArr := idBuilder.NewArray()
	defer idArr.Release()
	userArr := userIDBuilder.NewArray()
	defer userArr.Release()
	keyArr := keyBuilder.NewArray()
	defer keyArr.Release()
	valueArr := valueBuilder.NewArray()
	defer valueArr.Release()
	createdArr := createdAtBuilder.NewArray()
	defer createdArr.Release()
	updatedArr := updatedAtBuilder.NewArray()
	defer updatedArr.Release()

	columns := []arrow.Array{
		idArr, userArr, keyArr, valueArr, embeddingArray, createdArr, updatedArr,
	}

	return array.NewRecord(arrowSchema, columns, int64(len(memories))), nil
}

func mapToSpecialMemory(row map[string]interface{}) *SpecialMemory {
	var mem SpecialMemory
	if val, ok := row["id"].(string); ok {
		mem.ID = val
	}
	if val, ok := row["user_id"].(string); ok {
		mem.UserID = val
	}
	if val, ok := row["key"].(string); ok {
		mem.Key = val
	}
	if val, ok := row["value"].(string); ok {
		mem.Value = val
	}
	if val, ok := row["created_at"].(string); ok {
		mem.CreatedAt, _ = time.Parse(time.RFC3339Nano, val)
	}
	if val, ok := row["updated_at"].(string); ok {
		mem.UpdatedAt, _ = time.Parse(time.RFC3339Nano, val)
	}

	if val, ok := row["embedding"].([]float32); ok {
		mem.Embedding = val
	} else if val, ok := row["embedding"].([]interface{}); ok {
		emb := make([]float32, len(val))
		for i, v := range val {
			if f, ok := v.(float32); ok {
				emb[i] = f
			} else if f, ok := v.(float64); ok {
				emb[i] = float32(f)
			}
		}
		mem.Embedding = emb
	}

	return &mem
}

func (s *LanceDBStore) getConversationsTable(ctx context.Context) (contracts.ITable, error) {
	table, err := s.conn.OpenTable(ctx, "conversations")
	if err == nil {
		return table, nil
	}

	arrowSchema := getConversationArrowSchema()
	schema, err := lancedb.NewSchema(arrowSchema)
	if err != nil {
		return nil, fmt.Errorf("create conversations schema: %w", err)
	}

	table, err = s.conn.CreateTable(ctx, "conversations", schema)
	if err != nil {
		return nil, fmt.Errorf("create conversations table: %w", err)
	}
	return table, nil
}

func (s *LanceDBStore) getSpecialMemoriesTable(ctx context.Context, dim int) (contracts.ITable, error) {
	if dim <= 0 {
		dim = s.dim
	}
	table, err := s.conn.OpenTable(ctx, "special_memories")
	if err == nil {
		return table, nil
	}

	arrowSchema := getSpecialMemoryArrowSchema(dim)
	schema, err := lancedb.NewSchema(arrowSchema)
	if err != nil {
		return nil, fmt.Errorf("create special_memories schema: %w", err)
	}

	table, err = s.conn.CreateTable(ctx, "special_memories", schema)
	if err != nil {
		return nil, fmt.Errorf("create special_memories table: %w", err)
	}
	return table, nil
}

// SaveConversation persists a conversation turn.
func (s *LanceDBStore) SaveConversation(ctx context.Context, conv *Conversation) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if conv.ID == "" {
		conv.ID = uuid.New().String()
	}
	if conv.Timestamp.IsZero() {
		conv.Timestamp = time.Now().UTC()
	}

	table, err := s.getConversationsTable(ctx)
	if err != nil {
		return err
	}
	defer table.Close()

	record, err := conversationsToRecord([]*Conversation{conv})
	if err != nil {
		return err
	}
	defer record.Release()

	return table.Add(ctx, record, nil)
}

// SaveSpecialMemory persists or replaces a special memory.
func (s *LanceDBStore) SaveSpecialMemory(ctx context.Context, mem *SpecialMemory) error {
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

	dim := len(mem.Embedding)
	if dim == 0 {
		dim = s.dim
	}

	table, err := s.getSpecialMemoriesTable(ctx, dim)
	if err != nil {
		return err
	}
	defer table.Close()

	escapedID := strings.ReplaceAll(mem.ID, "'", "''")
	_ = table.Delete(ctx, fmt.Sprintf("id = '%s'", escapedID))

	record, err := specialMemoriesToRecord([]*SpecialMemory{mem}, dim)
	if err != nil {
		return err
	}
	defer record.Release()

	return table.Add(ctx, record, nil)
}

// ListSpecialMemories lists all special memories for a user.
func (s *LanceDBStore) ListSpecialMemories(ctx context.Context, userID string) ([]*SpecialMemory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	table, err := s.getSpecialMemoriesTable(ctx, s.dim)
	if err != nil {
		return nil, err
	}
	defer table.Close()

	escapedUserID := strings.ReplaceAll(userID, "'", "''")
	rows, err := table.SelectWithFilter(ctx, fmt.Sprintf("user_id = '%s'", escapedUserID))
	if err != nil {
		return nil, err
	}

	memories := make([]*SpecialMemory, len(rows))
	for i, row := range rows {
		memories[i] = mapToSpecialMemory(row)
	}

	sort.Slice(memories, func(i, j int) bool {
		return memories[i].CreatedAt.After(memories[j].CreatedAt)
	})

	return memories, nil
}

// DeleteSpecialMemory deletes a special memory turn by ID.
func (s *LanceDBStore) DeleteSpecialMemory(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	table, err := s.getSpecialMemoriesTable(ctx, s.dim)
	if err != nil {
		return err
	}
	defer table.Close()

	escapedID := strings.ReplaceAll(id, "'", "''")
	return table.Delete(ctx, fmt.Sprintf("id = '%s'", escapedID))
}

func mapToConversation(row map[string]interface{}) *Conversation {
	var conv Conversation
	if val, ok := row["id"].(string); ok {
		conv.ID = val
	}
	if val, ok := row["user_id"].(string); ok {
		conv.UserID = val
	}
	if val, ok := row["model"].(string); ok {
		conv.Model = val
	}
	if val, ok := row["user_message"].(string); ok {
		conv.UserMessage = val
	}
	if val, ok := row["assistant_message"].(string); ok {
		conv.AssistantMessage = val
	}
	if val, ok := row["thinking"].(string); ok {
		conv.Thinking = val
	}
	if val, ok := row["timestamp"].(string); ok {
		conv.Timestamp, _ = time.Parse(time.RFC3339Nano, val)
	}
	return &conv
}

// Export retrieves all records across all tables (memories, conversations, special_memories)
func (s *LanceDBStore) Export(ctx context.Context) ([]*Memory, []*Conversation, []*SpecialMemory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var memories []*Memory
	memTable, err := s.conn.OpenTable(ctx, "memories")
	if err == nil {
		defer memTable.Close()
		rows, err := memTable.Select(ctx, contracts.QueryConfig{})
		if err == nil {
			for _, row := range rows {
				memories = append(memories, mapToMemory(row))
			}
		}
	}

	var conversations []*Conversation
	convTable, err := s.conn.OpenTable(ctx, "conversations")
	if err == nil {
		defer convTable.Close()
		rows, err := convTable.Select(ctx, contracts.QueryConfig{})
		if err == nil {
			for _, row := range rows {
				conversations = append(conversations, mapToConversation(row))
			}
		}
	}

	var specialMemories []*SpecialMemory
	specTable, err := s.conn.OpenTable(ctx, "special_memories")
	if err == nil {
		defer specTable.Close()
		rows, err := specTable.Select(ctx, contracts.QueryConfig{})
		if err == nil {
			for _, row := range rows {
				specialMemories = append(specialMemories, mapToSpecialMemory(row))
			}
		}
	}

	return memories, conversations, specialMemories, nil
}

// Wipe clears all data in the store by closing the DB, deleting the directory, and reconnecting.
func (s *LanceDBStore) Wipe(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_ = s.conn.Close()

	if err := os.RemoveAll(s.dbDir); err != nil {
		return fmt.Errorf("wipe failed to delete directory: %w", err)
	}

	if err := os.MkdirAll(s.dbDir, 0o755); err != nil {
		return fmt.Errorf("wipe failed to recreate directory: %w", err)
	}

	conn, err := lancedb.Connect(ctx, s.dbDir, nil)
	if err != nil {
		return fmt.Errorf("wipe failed to reconnect: %w", err)
	}
	s.conn = conn
	return nil
}


