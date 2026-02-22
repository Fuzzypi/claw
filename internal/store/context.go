package store

import "time"

// ContextEntry represents a row in the context_entries table.
type ContextEntry struct {
	ID         int64
	PipelineID int64
	JobID      int64
	Content    string
	CreatedAt  time.Time
}

// AddContextEntry inserts a new context entry.
func (s *Store) AddContextEntry(pipelineID, jobID int64, content string) (*ContextEntry, error) {
	res, err := s.db.Exec(
		`INSERT INTO context_entries (pipeline_id, job_id, content) VALUES (?, ?, ?)`,
		pipelineID, jobID, content,
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	e := &ContextEntry{}
	err = s.db.QueryRow(
		`SELECT id, pipeline_id, job_id, content, created_at FROM context_entries WHERE id = ?`, id,
	).Scan(&e.ID, &e.PipelineID, &e.JobID, &e.Content, &e.CreatedAt)
	return e, err
}

// ListContextByPipeline returns all context entries for the given pipeline.
func (s *Store) ListContextByPipeline(pipelineID int64) ([]*ContextEntry, error) {
	rows, err := s.db.Query(
		`SELECT id, pipeline_id, job_id, content, created_at
		FROM context_entries WHERE pipeline_id = ? ORDER BY id`, pipelineID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*ContextEntry
	for rows.Next() {
		e := &ContextEntry{}
		if err := rows.Scan(&e.ID, &e.PipelineID, &e.JobID, &e.Content, &e.CreatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
