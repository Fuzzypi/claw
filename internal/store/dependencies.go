package store

import "fmt"

// AddDependency records that jobID depends on dependsOnID.
// Returns an error if the dependency would create a cycle.
func (s *Store) AddDependency(jobID, dependsOnID int64) error {
	if jobID == dependsOnID {
		return fmt.Errorf("job %d cannot depend on itself", jobID)
	}

	if err := s.detectCycle(jobID, dependsOnID); err != nil {
		return err
	}

	_, err := s.db.Exec(
		`INSERT INTO job_dependencies (job_id, depends_on) VALUES (?, ?)`,
		jobID, dependsOnID,
	)
	return err
}

// detectCycle checks whether adding dependsOnID -> jobID would form a cycle.
// A cycle exists if jobID can reach dependsOnID by following existing dependencies.
func (s *Store) detectCycle(jobID, dependsOnID int64) error {
	visited := map[int64]bool{}
	queue := []int64{dependsOnID}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if current == jobID {
			return fmt.Errorf("adding dependency %d -> %d would create a cycle", jobID, dependsOnID)
		}
		if visited[current] {
			continue
		}
		visited[current] = true

		rows, err := s.db.Query(
			`SELECT depends_on FROM job_dependencies WHERE job_id = ?`, current,
		)
		if err != nil {
			return err
		}
		for rows.Next() {
			var dep int64
			if err := rows.Scan(&dep); err != nil {
				rows.Close()
				return err
			}
			if !visited[dep] {
				queue = append(queue, dep)
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
	}

	return nil
}

// ListReadyJobs returns pending jobs whose dependencies are all completed.
func (s *Store) ListReadyJobs(pipelineID int64) ([]*Job, error) {
	rows, err := s.db.Query(
		`SELECT j.id, j.pipeline_id, j.name, j.prompt, j.status, j.agent_id,
			j.gate_command, j.gate_status, j.gate_exit_code, j.gate_output,
			j.output, j.exit_code, j.attempt_count, j.lease_expires_at,
			j.started_at, j.completed_at, j.created_at
		FROM jobs j
		WHERE j.pipeline_id = ?
			AND j.status = 'pending'
			AND NOT EXISTS (
				SELECT 1 FROM job_dependencies d
				JOIN jobs dep ON dep.id = d.depends_on
				WHERE d.job_id = j.id
					AND dep.status != 'completed'
			)
		ORDER BY j.id`, pipelineID,
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
