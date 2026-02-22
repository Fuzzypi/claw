package store

import (
	"database/sql"
	"fmt"
	"time"
)

// Pipeline represents a row in the pipelines table.
type Pipeline struct {
	ID          int64
	Name        string
	ProjectDir  string
	Status      string
	CreatedAt   time.Time
	CompletedAt *time.Time
}

// CreatePipeline inserts a new pipeline.
func (s *Store) CreatePipeline(name, projectDir string) (*Pipeline, error) {
	res, err := s.db.Exec(
		`INSERT INTO pipelines (name, project_dir) VALUES (?, ?)`,
		name, projectDir,
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.GetPipeline(id)
}

// GetPipeline retrieves a pipeline by ID.
func (s *Store) GetPipeline(id int64) (*Pipeline, error) {
	p := &Pipeline{}
	err := s.db.QueryRow(
		`SELECT id, name, project_dir, status, created_at, completed_at
		FROM pipelines WHERE id = ?`, id,
	).Scan(&p.ID, &p.Name, &p.ProjectDir, &p.Status, &p.CreatedAt, &p.CompletedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("pipeline %d not found", id)
	}
	return p, err
}

// ListPipelines returns all pipelines ordered by ID.
func (s *Store) ListPipelines() ([]*Pipeline, error) {
	rows, err := s.db.Query(
		`SELECT id, name, project_dir, status, created_at, completed_at
		FROM pipelines ORDER BY id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pipelines []*Pipeline
	for rows.Next() {
		p := &Pipeline{}
		if err := rows.Scan(&p.ID, &p.Name, &p.ProjectDir, &p.Status, &p.CreatedAt, &p.CompletedAt); err != nil {
			return nil, err
		}
		pipelines = append(pipelines, p)
	}
	return pipelines, rows.Err()
}

// UpdatePipelineStatus changes the status of a pipeline.
func (s *Store) UpdatePipelineStatus(id int64, status string) error {
	_, err := s.db.Exec(`UPDATE pipelines SET status = ? WHERE id = ?`, status, id)
	return err
}
