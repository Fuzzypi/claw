package dispatch

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	clawctx "github.com/fuzzypi/claw/internal/context"
	"github.com/fuzzypi/claw/internal/gates"
	"github.com/fuzzypi/claw/internal/store"
)

// Dispatcher assigns ready jobs to idle agents and manages execution.
type Dispatcher struct {
	store    *store.Store
	interval time.Duration
	mu       sync.Mutex
}

// NewDispatcher creates a new dispatcher with a 1-second poll interval.
func NewDispatcher(s *store.Store) *Dispatcher {
	return &Dispatcher{
		store:    s,
		interval: 1 * time.Second,
	}
}

func (d *Dispatcher) logActivity(pipelineID *int64, jobID *int64, agentID *int64, event, detail string) {
	if err := d.store.LogActivity(pipelineID, jobID, agentID, event, detail); err != nil {
		log.Printf("[claw] activity log error: %v", err)
	}
}

// Run executes the dispatch loop for the given pipeline until all jobs
// complete or a job fails after exhausting retries.
func (d *Dispatcher) Run(pipelineID int64) error {
	var wg sync.WaitGroup
	errCh := make(chan error, 1)

	d.logActivity(&pipelineID, nil, nil, "pipeline_started", "Pipeline execution started")

	for {
		// Recover stale jobs first
		d.RecoverStaleJobs(pipelineID)

		// Check terminal conditions
		jobs, err := d.store.ListJobsByPipeline(pipelineID)
		if err != nil {
			return fmt.Errorf("listing jobs: %w", err)
		}

		allDone := true
		var failedJob string
		for _, j := range jobs {
			switch j.Status {
			case "failed":
				failedJob = j.Name
			case "pending", "dispatched", "running":
				allDone = false
			}
		}

		if failedJob != "" {
			wg.Wait()
			err := fmt.Errorf("job %q failed", failedJob)
			d.logActivity(&pipelineID, nil, nil, "pipeline_failed", fmt.Sprintf("Pipeline failed: %v", err))
			return err
		}
		if allDone {
			wg.Wait()
			d.logActivity(&pipelineID, nil, nil, "pipeline_completed", "Pipeline completed successfully")
			return nil
		}

		// Get ready jobs and idle agents
		ready, err := d.store.ListReadyJobs(pipelineID)
		if err != nil {
			return fmt.Errorf("listing ready jobs: %w", err)
		}

		idle, err := d.store.ListAgentsByStatus("idle")
		if err != nil {
			return fmt.Errorf("listing idle agents: %w", err)
		}

		// Assign jobs to agents
		pairs := min(len(ready), len(idle))
		for i := 0; i < pairs; i++ {
			job := ready[i]
			agent := idle[i]

			leaseExpires := time.Now().Add(time.Duration(agent.TimeoutSecs) * time.Second)
			if err := d.store.AssignJobToAgent(job.ID, agent.ID, leaseExpires); err != nil {
				log.Printf("failed to assign job %q to agent %q: %v", job.Name, agent.Name, err)
				continue
			}

			// Re-read after assignment
			job, _ = d.store.GetJob(job.ID)
			agent, _ = d.store.GetAgent(agent.ID)

			d.logActivity(&pipelineID, &job.ID, &agent.ID, "job_dispatched",
				fmt.Sprintf("Job %q dispatched to agent %q", job.Name, agent.Name))

			if agent.Type == "manual" {
				// Manual agents run synchronously (block on stdin)
				if err := d.executeJob(job, agent); err != nil {
					select {
					case errCh <- err:
					default:
					}
				}
			} else {
				// Shell agents run in goroutines
				wg.Add(1)
				go func(j *store.Job, a *store.Agent) {
					defer wg.Done()
					if err := d.executeJob(j, a); err != nil {
						select {
						case errCh <- err:
						default:
						}
					}
				}(job, agent)
			}
		}

		time.Sleep(d.interval)
	}
}

func (d *Dispatcher) executeJob(job *store.Job, agent *store.Agent) error {
	d.mu.Lock()
	_ = d.store.UpdateJobStatus(job.ID, "running")
	d.mu.Unlock()

	var execErr error
	switch agent.Type {
	case "shell":
		execErr = ExecuteShell(d.store, job, agent)
	case "manual":
		execErr = ExecuteManual(d.store, job, agent, os.Stdin, os.Stdout)
	default:
		execErr = fmt.Errorf("%s executor not implemented", agent.Type)
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	// Always clear agent assignment
	_ = d.store.ClearAgentAssignment(agent.ID)
	_ = d.store.SetJobLeaseExpires(job.ID, nil)

	// Re-read job for current state
	job, _ = d.store.GetJob(job.ID)

	maxRetries := getMaxRetries()

	if execErr != nil {
		// Extract context even on failure
		d.extractAndStoreContext(job)

		exitCode := -1
		if job.ExitCode != nil {
			exitCode = *job.ExitCode
		}
		d.logActivity(&job.PipelineID, &job.ID, &agent.ID, "job_failed",
			fmt.Sprintf("Job %q failed (exit code %d)", job.Name, exitCode))

		if job.AttemptCount <= maxRetries {
			log.Printf("Retrying job %q (attempt %d/%d)", job.Name, job.AttemptCount, maxRetries+1)
			_ = d.store.UpdateJobStatus(job.ID, "pending")
		} else {
			log.Printf("Job %q failed after %d attempts", job.Name, job.AttemptCount)
			_ = d.store.UpdateJobStatus(job.ID, "failed")
			_ = d.store.SetJobCompleted(job.ID)
		}
		return nil
	}

	// Executor succeeded (exit code 0) — now run gate if configured
	gateFailed := false
	if job.GateCommand != nil && *job.GateCommand != "" {
		pipeline, err := d.store.GetPipeline(job.PipelineID)
		workDir := "/tmp"
		if err == nil {
			workDir = pipeline.ProjectDir
		}

		gateResult, gateErr := gates.RunGate(*job.GateCommand, workDir, time.Duration(agent.TimeoutSecs)*time.Second)
		if gateErr != nil {
			log.Printf("Gate error for job %q: %v", job.Name, gateErr)
			_ = d.store.SetJobGateResult(job.ID, gateErr.Error(), -1, "failed")
			gateFailed = true
			d.logActivity(&job.PipelineID, &job.ID, nil, "gate_failed",
				fmt.Sprintf("Gate %q FAILED (error: %v)", *job.GateCommand, gateErr))
		} else {
			status := "passed"
			if !gateResult.Passed {
				status = "failed"
				gateFailed = true
				d.logActivity(&job.PipelineID, &job.ID, nil, "gate_failed",
					fmt.Sprintf("Gate %q FAILED (exit code %d)", *job.GateCommand, gateResult.ExitCode))
			} else {
				d.logActivity(&job.PipelineID, &job.ID, nil, "gate_passed",
					fmt.Sprintf("Gate %q PASSED", *job.GateCommand))
			}
			_ = d.store.SetJobGateResult(job.ID, gateResult.Output, gateResult.ExitCode, status)
		}

		// Re-read job after gate result stored
		job, _ = d.store.GetJob(job.ID)
	}

	// Extract context (after gate result is stored)
	d.extractAndStoreContext(job)

	if gateFailed {
		if job.AttemptCount <= maxRetries {
			log.Printf("Retrying job %q (gate failed, attempt %d/%d)", job.Name, job.AttemptCount, maxRetries+1)
			_ = d.store.UpdateJobStatus(job.ID, "pending")
		} else {
			log.Printf("Job %q failed after %d attempts (gate failed)", job.Name, job.AttemptCount)
			_ = d.store.UpdateJobStatus(job.ID, "failed")
			_ = d.store.SetJobCompleted(job.ID)
		}
		return nil
	}

	// Success
	exitCode := 0
	if job.ExitCode != nil {
		exitCode = *job.ExitCode
	}
	d.logActivity(&job.PipelineID, &job.ID, &agent.ID, "job_completed",
		fmt.Sprintf("Job %q completed (exit code %d)", job.Name, exitCode))

	_ = d.store.UpdateJobStatus(job.ID, "completed")
	_ = d.store.SetJobCompleted(job.ID)
	return nil
}

func (d *Dispatcher) extractAndStoreContext(job *store.Job) {
	output := ""
	if job.Output != nil {
		output = *job.Output
	}
	exitCode := 0
	if job.ExitCode != nil {
		exitCode = *job.ExitCode
	}
	gateStatus := ""
	if job.GateStatus != nil {
		gateStatus = *job.GateStatus
	}

	ctxStr := clawctx.ExtractContext(job.Name, output, exitCode, gateStatus)
	_, _ = d.store.AddContextEntry(job.PipelineID, job.ID, ctxStr)
}

// RecoverStaleJobs resets timed-out jobs and frees their agents.
func (d *Dispatcher) RecoverStaleJobs(pipelineID int64) {
	stale, err := d.store.GetStaleJobs()
	if err != nil {
		log.Printf("failed to get stale jobs: %v", err)
		return
	}

	maxRetries := getMaxRetries()

	for _, job := range stale {
		if job.PipelineID != pipelineID {
			continue
		}

		_ = d.store.SetJobOutput(job.ID,
			fmt.Sprintf("[claw] job timed out after %d seconds", job.AttemptCount),
			-1)
		_ = d.store.SetJobLeaseExpires(job.ID, nil)

		if job.AgentID != nil {
			_ = d.store.ClearAgentAssignment(*job.AgentID)
		}

		d.logActivity(&pipelineID, &job.ID, nil, "job_timeout",
			fmt.Sprintf("Job %q timed out, attempt %d", job.Name, job.AttemptCount))

		if job.AttemptCount <= maxRetries {
			log.Printf("Retrying job %q (attempt %d/%d)", job.Name, job.AttemptCount, maxRetries+1)
			_ = d.store.UpdateJobStatus(job.ID, "pending")
		} else {
			log.Printf("Job %q failed after %d attempts", job.Name, job.AttemptCount)
			_ = d.store.UpdateJobStatus(job.ID, "failed")
			_ = d.store.SetJobCompleted(job.ID)
		}
	}
}

func getMaxRetries() int {
	s := os.Getenv("CLAW_MAX_RETRIES")
	if s == "" {
		return 1
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 1
	}
	return n
}
