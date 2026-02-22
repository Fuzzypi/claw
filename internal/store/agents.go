package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// Agent represents a row in the agents table.
type Agent struct {
	ID           int64
	Name         string
	Type         string
	Command      *string
	Args         []string
	Cwd          *string
	TimeoutSecs  int
	Status       string
	CurrentJobID *int64
	RegisteredAt time.Time
}

// RegisterAgent inserts a new agent.
func (s *Store) RegisterAgent(name, agentType string, command *string, args []string, cwd *string, timeoutSecs *int) (*Agent, error) {
	var argsJSON *string
	if args != nil {
		b, err := json.Marshal(args)
		if err != nil {
			return nil, err
		}
		s := string(b)
		argsJSON = &s
	}

	timeout := 600
	if timeoutSecs != nil {
		timeout = *timeoutSecs
	}

	res, err := s.db.Exec(
		`INSERT INTO agents (name, type, command, args, cwd, timeout_secs) VALUES (?, ?, ?, ?, ?, ?)`,
		name, agentType, command, argsJSON, cwd, timeout,
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.GetAgent(id)
}

// GetAgent retrieves an agent by ID.
func (s *Store) GetAgent(id int64) (*Agent, error) {
	a := &Agent{}
	var argsJSON *string
	err := s.db.QueryRow(
		`SELECT id, name, type, command, args, cwd, timeout_secs, status, current_job_id, registered_at
		FROM agents WHERE id = ?`, id,
	).Scan(&a.ID, &a.Name, &a.Type, &a.Command, &argsJSON, &a.Cwd, &a.TimeoutSecs, &a.Status, &a.CurrentJobID, &a.RegisteredAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("agent %d not found", id)
	}
	if err != nil {
		return nil, err
	}
	if argsJSON != nil {
		if err := json.Unmarshal([]byte(*argsJSON), &a.Args); err != nil {
			return nil, err
		}
	}
	return a, nil
}

// ListAgents returns all agents ordered by ID.
func (s *Store) ListAgents() ([]*Agent, error) {
	rows, err := s.db.Query(
		`SELECT id, name, type, command, args, cwd, timeout_secs, status, current_job_id, registered_at
		FROM agents ORDER BY id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []*Agent
	for rows.Next() {
		a := &Agent{}
		var argsJSON *string
		if err := rows.Scan(&a.ID, &a.Name, &a.Type, &a.Command, &argsJSON, &a.Cwd, &a.TimeoutSecs, &a.Status, &a.CurrentJobID, &a.RegisteredAt); err != nil {
			return nil, err
		}
		if argsJSON != nil {
			if err := json.Unmarshal([]byte(*argsJSON), &a.Args); err != nil {
				return nil, err
			}
		}
		agents = append(agents, a)
	}
	return agents, rows.Err()
}

// UpdateAgentStatus changes the status and current job of an agent.
func (s *Store) UpdateAgentStatus(id int64, status string, currentJobID *int64) error {
	_, err := s.db.Exec(
		`UPDATE agents SET status = ?, current_job_id = ? WHERE id = ?`,
		status, currentJobID, id,
	)
	return err
}

// ListAgentsByStatus returns all agents with the given status.
func (s *Store) ListAgentsByStatus(status string) ([]*Agent, error) {
	rows, err := s.db.Query(
		`SELECT id, name, type, command, args, cwd, timeout_secs, status, current_job_id, registered_at
		FROM agents WHERE status = ? ORDER BY id`, status,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []*Agent
	for rows.Next() {
		a := &Agent{}
		var argsJSON *string
		if err := rows.Scan(&a.ID, &a.Name, &a.Type, &a.Command, &argsJSON, &a.Cwd, &a.TimeoutSecs, &a.Status, &a.CurrentJobID, &a.RegisteredAt); err != nil {
			return nil, err
		}
		if argsJSON != nil {
			if err := json.Unmarshal([]byte(*argsJSON), &a.Args); err != nil {
				return nil, err
			}
		}
		agents = append(agents, a)
	}
	return agents, rows.Err()
}

// ClearAgentAssignment sets an agent back to idle with no current job.
func (s *Store) ClearAgentAssignment(agentID int64) error {
	_, err := s.db.Exec(
		`UPDATE agents SET status = 'idle', current_job_id = NULL WHERE id = ?`,
		agentID,
	)
	return err
}
