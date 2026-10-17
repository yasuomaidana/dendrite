package segmented

import (
	"Dendrite/pkg/statedir"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SQLiteIndex manages the relational metadata index for segments and checkpoints.
type SQLiteIndex struct {
	dbPath string
	mu     sync.RWMutex
}

// NewSQLiteIndex creates a new SQLiteIndex storing its meta.db at dbPath.
func NewSQLiteIndex(dbPath string) (*SQLiteIndex, error) {
	if dbPath == "" {
		dbPath = ".dendrite/store/meta.db"
	}
	dbPath = statedir.Resolve(dbPath)

	idx := &SQLiteIndex{dbPath: dbPath}
	if err := idx.initSchema(); err != nil {
		return nil, fmt.Errorf("initializing sqlite schema: %w", err)
	}
	return idx, nil
}

func (idx *SQLiteIndex) initSchema() error {
	dir := filepath.Dir(idx.dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	schema := `
PRAGMA journal_mode = WAL;
CREATE TABLE IF NOT EXISTS segments (
    hash TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    identifier TEXT NOT NULL,
    project TEXT NOT NULL,
    size_bytes INTEGER NOT NULL,
    comp_size INTEGER NOT NULL,
    blob_path TEXT NOT NULL,
    metadata TEXT,
    created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS checkpoints (
    chunk_id TEXT PRIMARY KEY,
    project TEXT NOT NULL,
    state TEXT NOT NULL,
    total_segments INTEGER NOT NULL,
    committed_segments INTEGER NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS chunk_segments (
    chunk_id TEXT NOT NULL,
    segment_hash TEXT NOT NULL,
    PRIMARY KEY (chunk_id, segment_hash)
);
`
	return idx.execSQL(schema)
}

// SaveCheckpointTx saves a checkpoint and its associated segments within an atomic ACID transaction.
func (idx *SQLiteIndex) SaveCheckpointTx(cp ChunkCheckpoint, segments []Segment) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	var sb strings.Builder
	sb.WriteString("BEGIN TRANSACTION;\n")

	// 1. Insert or replace checkpoint
	sb.WriteString(fmt.Sprintf(
		"INSERT OR REPLACE INTO checkpoints (chunk_id, project, state, total_segments, committed_segments, updated_at) VALUES ('%s', '%s', '%s', %d, %d, '%s');\n",
		escapeSQL(cp.ChunkID), escapeSQL(cp.Project), escapeSQL(string(cp.State)), cp.TotalSegments, cp.CommittedSegments, cp.UpdatedAt.Format(time.RFC3339),
	))

	// 2. Insert segments and chunk-segment links
	for _, s := range segments {
		metaJSON, _ := json.Marshal(s.Metadata)
		sb.WriteString(fmt.Sprintf(
			"INSERT OR REPLACE INTO segments (hash, type, identifier, project, size_bytes, comp_size, blob_path, metadata, created_at) VALUES ('%s', '%s', '%s', '%s', %d, %d, '%s', '%s', '%s');\n",
			escapeSQL(s.Hash), escapeSQL(string(s.Type)), escapeSQL(s.Identifier), escapeSQL(s.Project), s.SizeBytes, s.CompressedSize, escapeSQL(s.BlobPath), escapeSQL(string(metaJSON)), s.CreatedAt.Format(time.RFC3339),
		))
		sb.WriteString(fmt.Sprintf(
			"INSERT OR IGNORE INTO chunk_segments (chunk_id, segment_hash) VALUES ('%s', '%s');\n",
			escapeSQL(cp.ChunkID), escapeSQL(s.Hash),
		))
	}

	sb.WriteString("COMMIT;\n")
	return idx.execSQL(sb.String())
}

// GetSegment retrieves a single segment by hash.
func (idx *SQLiteIndex) GetSegment(hash string) (*Segment, error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	query := fmt.Sprintf("SELECT hash, type, identifier, project, size_bytes, comp_size, blob_path, metadata, created_at FROM segments WHERE hash = '%s';", escapeSQL(hash))
	rows, err := idx.queryJSON(query)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrSegmentNotFound
	}
	return parseSegmentRow(rows[0]), nil
}

// ListSegments returns all segments registered for a given project.
func (idx *SQLiteIndex) ListSegments(project string) ([]Segment, error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var query string
	if project == "" {
		query = "SELECT hash, type, identifier, project, size_bytes, comp_size, blob_path, metadata, created_at FROM segments ORDER BY created_at ASC;"
	} else {
		query = fmt.Sprintf("SELECT hash, type, identifier, project, size_bytes, comp_size, blob_path, metadata, created_at FROM segments WHERE project = '%s' ORDER BY created_at ASC;", escapeSQL(project))
	}

	rows, err := idx.queryJSON(query)
	if err != nil {
		return nil, err
	}

	var results []Segment
	for _, r := range rows {
		results = append(results, *parseSegmentRow(r))
	}
	return results, nil
}

// GetCheckpoint retrieves a checkpoint by chunk ID.
func (idx *SQLiteIndex) GetCheckpoint(chunkID string) (*ChunkCheckpoint, error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	query := fmt.Sprintf("SELECT chunk_id, project, state, total_segments, committed_segments, updated_at FROM checkpoints WHERE chunk_id = '%s';", escapeSQL(chunkID))
	rows, err := idx.queryJSON(query)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrCheckpointNotFound
	}
	return parseCheckpointRow(rows[0]), nil
}

// ListCheckpoints returns all checkpoints for a project.
func (idx *SQLiteIndex) ListCheckpoints(project string) ([]ChunkCheckpoint, error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var query string
	if project == "" {
		query = "SELECT chunk_id, project, state, total_segments, committed_segments, updated_at FROM checkpoints ORDER BY updated_at ASC;"
	} else {
		query = fmt.Sprintf("SELECT chunk_id, project, state, total_segments, committed_segments, updated_at FROM checkpoints WHERE project = '%s' ORDER BY updated_at ASC;", escapeSQL(project))
	}

	rows, err := idx.queryJSON(query)
	if err != nil {
		return nil, err
	}

	var results []ChunkCheckpoint
	for _, r := range rows {
		results = append(results, *parseCheckpointRow(r))
	}
	return results, nil
}

// PurgeProject explicitly deletes all checkpoints and segments for a project.
func (idx *SQLiteIndex) PurgeProject(project string) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	stmt := fmt.Sprintf(`
BEGIN TRANSACTION;
DELETE FROM chunk_segments WHERE chunk_id IN (SELECT chunk_id FROM checkpoints WHERE project = '%s');
DELETE FROM checkpoints WHERE project = '%s';
DELETE FROM segments WHERE project = '%s';
COMMIT;
`, escapeSQL(project), escapeSQL(project), escapeSQL(project))
	return idx.execSQL(stmt)
}

func (idx *SQLiteIndex) execSQL(sqlStmt string) error {
	cmd := exec.Command("sqlite3", idx.dbPath, sqlStmt)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sqlite error (%v): %s", err, string(output))
	}
	return nil
}

func (idx *SQLiteIndex) queryJSON(sqlQuery string) ([]map[string]interface{}, error) {
	cmd := exec.Command("sqlite3", "-json", idx.dbPath, sqlQuery)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("sqlite query error (%v): %s", err, string(output))
	}
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return []map[string]interface{}{}, nil
	}
	var rows []map[string]interface{}
	if err := json.Unmarshal([]byte(trimmed), &rows); err != nil {
		return nil, fmt.Errorf("parsing sqlite json: %w", err)
	}
	return rows, nil
}

func parseSegmentRow(r map[string]interface{}) *Segment {
	s := &Segment{
		Hash:       fmt.Sprintf("%v", r["hash"]),
		Type:       SegmentType(fmt.Sprintf("%v", r["type"])),
		Identifier: fmt.Sprintf("%v", r["identifier"]),
		Project:    fmt.Sprintf("%v", r["project"]),
		BlobPath:   fmt.Sprintf("%v", r["blob_path"]),
	}
	if sz, ok := r["size_bytes"].(float64); ok {
		s.SizeBytes = int64(sz)
	}
	if csz, ok := r["comp_size"].(float64); ok {
		s.CompressedSize = int64(csz)
	}
	if dtStr, ok := r["created_at"].(string); ok {
		s.CreatedAt, _ = time.Parse(time.RFC3339, dtStr)
	}
	if metaStr, ok := r["metadata"].(string); ok && metaStr != "" {
		_ = json.Unmarshal([]byte(metaStr), &s.Metadata)
	}
	return s
}

func parseCheckpointRow(r map[string]interface{}) *ChunkCheckpoint {
	cp := &ChunkCheckpoint{
		ChunkID: fmt.Sprintf("%v", r["chunk_id"]),
		Project: fmt.Sprintf("%v", r["project"]),
		State:   ChunkState(fmt.Sprintf("%v", r["state"])),
	}
	if tot, ok := r["total_segments"].(float64); ok {
		cp.TotalSegments = int(tot)
	}
	if com, ok := r["committed_segments"].(float64); ok {
		cp.CommittedSegments = int(com)
	}
	if cp.TotalSegments > 0 {
		cp.PercentComplete = (float64(cp.CommittedSegments) / float64(cp.TotalSegments)) * 100
	}
	if dtStr, ok := r["updated_at"].(string); ok {
		cp.UpdatedAt, _ = time.Parse(time.RFC3339, dtStr)
	}
	return cp
}

func escapeSQL(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
