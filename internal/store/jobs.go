package store

import (
	"database/sql"
	"fmt"
	"time"
)

// Job represents a row in the jobs table.
type Job struct {
	ID             int64
	PipelineID     int64
	Name           string
	Prompt         string
	Status         string
	AgentID        *int64
	GateCommand    *string
	GateStatus     *string
	GateExitCode   *int
	GateOutput     *string
	Output         *string
	ExitCode       *int
	AttemptCount   int
	LeaseExpiresAt *time.Time
	StartedAt      *time.Time
	CompletedAt    *time.Time
	CreatedAt      time.Time
}

const maxOutputBytes = 1_048_576
const keepBytes = 262_144

// CreateJob inserts a new job into the given pipeline.
func (s *Store) CreateJob(pipelineID int64, name, prompt string) (*Job, error) {
	res, err := s.db.Exec(
		`INSERT INTO jobs (pipeline_id, name, prompt) VALUES (?, ?, ?)`,
		pipelineID, name, prompt,
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.GetJob(id)
}

// GetJob retrieves a job by ID.
func (s *Store) GetJob(id int64) (*Job, error) {
	j := &Job{}
	err := s.db.QueryRow(
		`SELECT id, pipeline_id, name, prompt, status, agent_id,
			gate_command, gate_status, gate_exit_code, gate_output,
			output, exit_code, attempt_count, lease_expires_at,
			started_at, completed_at, created_at
		FROM jobs WHERE id = ?`, id,
	).Scan(
		&j.ID, &j.PipelineID, &j.Name, &j.Prompt, &j.Status, &j.AgentID,
		&j.GateCommand, &j.GateStatus, &j.GateExitCode, &j.GateOutput,
		&j.Output, &j.ExitCode, &j.AttemptCount, &j.LeaseExpiresAt,
		&j.StartedAt, &j.CompletedAt, &j.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("job %d not found", id)
	}
	return j, err
}

// ListJobsByPipeline returns all jobs belonging to the given pipeline.
func (s *Store) ListJobsByPipeline(pipelineID int64) ([]*Job, error) {
	rows, err := s.db.Query(
		`SELECT id, pipeline_id, name, prompt, status, agent_id,
			gate_command, gate_status, gate_exit_code, gate_output,
			output, exit_code, attempt_count, lease_expires_at,
			started_at, completed_at, created_at
		FROM jobs WHERE pipeline_id = ? ORDER BY id`, pipelineID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []*Job
	for rows.Next() {
		j := &Job{}
		if err := rows.Scan(
			&j.ID, &j.PipelineID, &j.Name, &j.Prompt, &j.Status, &j.AgentID,
			&j.GateCommand, &j.GateStatus, &j.GateExitCode, &j.GateOutput,
			&j.Output, &j.ExitCode, &j.AttemptCount, &j.LeaseExpiresAt,
			&j.StartedAt, &j.CompletedAt, &j.CreatedAt,
		); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// UpdateJobStatus changes the status of a job.
func (s *Store) UpdateJobStatus(id int64, status string) error {
	_, err := s.db.Exec(`UPDATE jobs SET status = ? WHERE id = ?`, status, id)
	return err
}

// SetJobOutput stores the output and exit code for a job.
// Output exceeding 1MB is truncated: first 256KB + marker + last 256KB.
func (s *Store) SetJobOutput(id int64, output string, exitCode int) error {
	output = truncateOutput(output)
	_, err := s.db.Exec(
		`UPDATE jobs SET output = ?, exit_code = ? WHERE id = ?`,
		output, exitCode, id,
	)
	return err
}

// AssignJobToAgent dispatches a job to an agent, updating both records atomically.
func (s *Store) AssignJobToAgent(jobID, agentID int64, leaseExpires time.Time) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		`UPDATE jobs SET agent_id = ?, status = 'dispatched',
		started_at = CURRENT_TIMESTAMP,
		attempt_count = attempt_count + 1,
		lease_expires_at = ?
		WHERE id = ?`,
		agentID, leaseExpires, jobID,
	)
	if err != nil {
		return err
	}

	_, err = tx.Exec(
		`UPDATE agents SET status = 'busy', current_job_id = ? WHERE id = ?`,
		jobID, agentID,
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// GetStaleJobs returns jobs that are dispatched or running with an expired lease.
func (s *Store) GetStaleJobs() ([]*Job, error) {
	rows, err := s.db.Query(
		`SELECT id, pipeline_id, name, prompt, status, agent_id,
			gate_command, gate_status, gate_exit_code, gate_output,
			output, exit_code, attempt_count, lease_expires_at,
			started_at, completed_at, created_at
		FROM jobs
		WHERE status IN ('dispatched', 'running')
		AND lease_expires_at IS NOT NULL
		AND lease_expires_at < CURRENT_TIMESTAMP`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []*Job
	for rows.Next() {
		j := &Job{}
		if err := rows.Scan(
			&j.ID, &j.PipelineID, &j.Name, &j.Prompt, &j.Status, &j.AgentID,
			&j.GateCommand, &j.GateStatus, &j.GateExitCode, &j.GateOutput,
			&j.Output, &j.ExitCode, &j.AttemptCount, &j.LeaseExpiresAt,
			&j.StartedAt, &j.CompletedAt, &j.CreatedAt,
		); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// SetJobLeaseExpires sets or clears the lease expiration for a job.
func (s *Store) SetJobLeaseExpires(jobID int64, t *time.Time) error {
	_, err := s.db.Exec(`UPDATE jobs SET lease_expires_at = ? WHERE id = ?`, t, jobID)
	return err
}

// SetJobStarted sets the started_at timestamp to now.
func (s *Store) SetJobStarted(jobID int64) error {
	_, err := s.db.Exec(`UPDATE jobs SET started_at = CURRENT_TIMESTAMP WHERE id = ?`, jobID)
	return err
}

// SetJobCompleted sets the completed_at timestamp to now.
func (s *Store) SetJobCompleted(jobID int64) error {
	_, err := s.db.Exec(`UPDATE jobs SET completed_at = CURRENT_TIMESTAMP WHERE id = ?`, jobID)
	return err
}

// SetJobGateResult stores the gate execution result for a job.
func (s *Store) SetJobGateResult(jobID int64, output string, exitCode int, status string) error {
	output = truncateOutput(output)
	_, err := s.db.Exec(
		`UPDATE jobs SET gate_output = ?, gate_exit_code = ?, gate_status = ? WHERE id = ?`,
		output, exitCode, status, jobID,
	)
	return err
}

// SetJobGateCommand sets the gate command for a job.
func (s *Store) SetJobGateCommand(jobID int64, gateCommand string) error {
	_, err := s.db.Exec(`UPDATE jobs SET gate_command = ? WHERE id = ?`, gateCommand, jobID)
	return err
}

// GetJobByName retrieves a job by name within a pipeline.
func (s *Store) GetJobByName(pipelineID int64, name string) (*Job, error) {
	j := &Job{}
	err := s.db.QueryRow(
		`SELECT id, pipeline_id, name, prompt, status, agent_id,
			gate_command, gate_status, gate_exit_code, gate_output,
			output, exit_code, attempt_count, lease_expires_at,
			started_at, completed_at, created_at
		FROM jobs WHERE pipeline_id = ? AND name = ? LIMIT 1`, pipelineID, name,
	).Scan(
		&j.ID, &j.PipelineID, &j.Name, &j.Prompt, &j.Status, &j.AgentID,
		&j.GateCommand, &j.GateStatus, &j.GateExitCode, &j.GateOutput,
		&j.Output, &j.ExitCode, &j.AttemptCount, &j.LeaseExpiresAt,
		&j.StartedAt, &j.CompletedAt, &j.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("job %q not found in pipeline %d", name, pipelineID)
	}
	return j, err
}

func truncateOutput(output string) string {
	if len(output) <= maxOutputBytes {
		return output
	}
	dropped := len(output) - 2*keepBytes
	return output[:keepBytes] +
		fmt.Sprintf("\n[... truncated %d bytes ...]\n", dropped) +
		output[len(output)-keepBytes:]
}
