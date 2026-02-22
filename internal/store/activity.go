package store

import "time"

// ActivityEntry represents a row in the activity_log table.
type ActivityEntry struct {
	ID         int64
	PipelineID *int64
	JobID      *int64
	AgentID    *int64
	Event      string
	Detail     string
	CreatedAt  time.Time
}

// LogActivity inserts a new activity log entry.
func (s *Store) LogActivity(pipelineID *int64, jobID *int64, agentID *int64, event string, detail string) error {
	_, err := s.db.Exec(
		`INSERT INTO activity_log (pipeline_id, job_id, agent_id, event, detail) VALUES (?, ?, ?, ?, ?)`,
		pipelineID, jobID, agentID, event, detail,
	)
	return err
}

// ListActivity returns activity log entries, optionally filtered by pipeline.
// Results are ordered newest first, limited to the given count.
func (s *Store) ListActivity(pipelineID *int64, limit int) ([]*ActivityEntry, error) {
	if limit <= 0 {
		limit = 50
	}

	var query string
	var args []interface{}

	if pipelineID != nil {
		query = `SELECT id, pipeline_id, job_id, agent_id, event, detail, created_at
			FROM activity_log WHERE pipeline_id = ? ORDER BY created_at DESC, id DESC LIMIT ?`
		args = []interface{}{*pipelineID, limit}
	} else {
		query = `SELECT id, pipeline_id, job_id, agent_id, event, detail, created_at
			FROM activity_log ORDER BY created_at DESC, id DESC LIMIT ?`
		args = []interface{}{limit}
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*ActivityEntry
	for rows.Next() {
		e := &ActivityEntry{}
		if err := rows.Scan(&e.ID, &e.PipelineID, &e.JobID, &e.AgentID, &e.Event, &e.Detail, &e.CreatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
