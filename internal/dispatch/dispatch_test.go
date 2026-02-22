package dispatch

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fuzzypi/claw/internal/store"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }

func TestShellExecutorBasic(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")
	j, _ := s.CreateJob(p.ID, "echo-test", "hello world")
	a, _ := s.RegisterAgent("a1", "shell", strPtr("cat"), nil, nil, nil)

	err := ExecuteShell(s, j, a)
	if err != nil {
		t.Fatalf("ExecuteShell: %v", err)
	}

	got, _ := s.GetJob(j.ID)
	if got.Output == nil || !strings.Contains(*got.Output, "hello world") {
		t.Errorf("output = %v, want containing 'hello world'", got.Output)
	}
	if got.ExitCode == nil || *got.ExitCode != 0 {
		t.Errorf("exit code = %v, want 0", got.ExitCode)
	}
}

func TestShellExecutorExitCode(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")
	j, _ := s.CreateJob(p.ID, "fail-test", "ignored")
	a, _ := s.RegisterAgent("a1", "shell", strPtr("sh"), []string{"-c", "exit 42"}, nil, nil)

	err := ExecuteShell(s, j, a)
	if err == nil {
		t.Fatal("expected error for non-zero exit code")
	}

	got, _ := s.GetJob(j.ID)
	if got.ExitCode == nil || *got.ExitCode != 42 {
		t.Errorf("exit code = %v, want 42", got.ExitCode)
	}
}

func TestShellExecutorTimeout(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")
	j, _ := s.CreateJob(p.ID, "sleep-test", "ignored")
	timeout := 1
	a, _ := s.RegisterAgent("a1", "shell", strPtr("sleep"), []string{"10"}, nil, &timeout)

	start := time.Now()
	err := ExecuteShell(s, j, a)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error for timeout")
	}
	if elapsed > 5*time.Second {
		t.Errorf("took %v, expected < 5s", elapsed)
	}

	got, _ := s.GetJob(j.ID)
	if got.ExitCode == nil || *got.ExitCode == 0 {
		t.Errorf("exit code = %v, want non-zero", got.ExitCode)
	}
}

func TestShellExecutorOutputTruncation(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")
	j, _ := s.CreateJob(p.ID, "big-output", "ignored")
	a, _ := s.RegisterAgent("a1", "shell", strPtr("sh"),
		[]string{"-c", "dd if=/dev/zero bs=1 count=2000000 2>/dev/null | tr '\\0' 'x'"},
		nil, intPtr(30))

	err := ExecuteShell(s, j, a)
	if err != nil {
		t.Fatalf("ExecuteShell: %v", err)
	}

	got, _ := s.GetJob(j.ID)
	if got.Output == nil {
		t.Fatal("output is nil")
	}
	if len(*got.Output) > 1_100_000 {
		t.Errorf("output len = %d, want < 1.1MB", len(*got.Output))
	}
	if !strings.Contains(*got.Output, "[... truncated") {
		t.Error("truncation marker not found")
	}
}

func TestListAgentsByStatus(t *testing.T) {
	s := newTestStore(t)
	s.RegisterAgent("a1", "shell", nil, nil, nil, nil)
	s.RegisterAgent("a2", "shell", nil, nil, nil, nil)
	a3, _ := s.RegisterAgent("a3", "shell", nil, nil, nil, nil)

	// Set a3 to busy
	p, _ := s.CreatePipeline("p", "/tmp")
	j, _ := s.CreateJob(p.ID, "j", "pj")
	jobID := j.ID
	s.UpdateAgentStatus(a3.ID, "busy", &jobID)

	idle, _ := s.ListAgentsByStatus("idle")
	if len(idle) != 2 {
		t.Errorf("idle count = %d, want 2", len(idle))
	}

	busy, _ := s.ListAgentsByStatus("busy")
	if len(busy) != 1 {
		t.Errorf("busy count = %d, want 1", len(busy))
	}
}

func TestAssignJobToAgent(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")
	j, _ := s.CreateJob(p.ID, "j", "pj")
	a, _ := s.RegisterAgent("a1", "shell", nil, nil, nil, nil)

	lease := time.Now().Add(10 * time.Minute)
	if err := s.AssignJobToAgent(j.ID, a.ID, lease); err != nil {
		t.Fatalf("AssignJobToAgent: %v", err)
	}

	got, _ := s.GetJob(j.ID)
	if got.Status != "dispatched" {
		t.Errorf("job status = %q, want 'dispatched'", got.Status)
	}
	if got.AgentID == nil || *got.AgentID != a.ID {
		t.Errorf("agent_id = %v, want %d", got.AgentID, a.ID)
	}
	if got.AttemptCount != 1 {
		t.Errorf("attempt_count = %d, want 1", got.AttemptCount)
	}

	agent, _ := s.GetAgent(a.ID)
	if agent.Status != "busy" {
		t.Errorf("agent status = %q, want 'busy'", agent.Status)
	}
	if agent.CurrentJobID == nil || *agent.CurrentJobID != j.ID {
		t.Errorf("current_job_id = %v, want %d", agent.CurrentJobID, j.ID)
	}
}

func TestClearAgentAssignment(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")
	j, _ := s.CreateJob(p.ID, "j", "pj")
	a, _ := s.RegisterAgent("a1", "shell", nil, nil, nil, nil)

	lease := time.Now().Add(10 * time.Minute)
	s.AssignJobToAgent(j.ID, a.ID, lease)

	if err := s.ClearAgentAssignment(a.ID); err != nil {
		t.Fatalf("ClearAgentAssignment: %v", err)
	}

	agent, _ := s.GetAgent(a.ID)
	if agent.Status != "idle" {
		t.Errorf("status = %q, want 'idle'", agent.Status)
	}
	if agent.CurrentJobID != nil {
		t.Errorf("current_job_id = %v, want nil", agent.CurrentJobID)
	}
}

func TestGetStaleJobs(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")
	j1, _ := s.CreateJob(p.ID, "stale", "pj")
	a, _ := s.RegisterAgent("a1", "shell", nil, nil, nil, nil)

	// Assign with lease in the past
	pastLease := time.Now().Add(-1 * time.Second)
	s.AssignJobToAgent(j1.ID, a.ID, pastLease)

	stale, err := s.GetStaleJobs()
	if err != nil {
		t.Fatalf("GetStaleJobs: %v", err)
	}
	if len(stale) != 1 {
		t.Fatalf("stale count = %d, want 1", len(stale))
	}

	// Create another job with future lease
	j2, _ := s.CreateJob(p.ID, "fresh", "pj")
	s.ClearAgentAssignment(a.ID)
	futureLease := time.Now().Add(1 * time.Hour)
	s.AssignJobToAgent(j2.ID, a.ID, futureLease)

	stale, _ = s.GetStaleJobs()
	// j1 is still stale (dispatched, past lease), j2 is not
	staleCount := 0
	for _, sj := range stale {
		if sj.ID == j1.ID {
			staleCount++
		}
	}
	if staleCount != 1 {
		t.Errorf("stale j1 count = %d, want 1", staleCount)
	}
}

func TestDispatcherSequentialPipeline(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")
	a, _ := s.CreateJob(p.ID, "A", "output-A")
	b, _ := s.CreateJob(p.ID, "B", "output-B")
	c, _ := s.CreateJob(p.ID, "C", "output-C")

	s.AddDependency(b.ID, a.ID)
	s.AddDependency(c.ID, b.ID)

	s.RegisterAgent("agent-1", "shell", strPtr("cat"), nil, nil, nil)

	d := NewDispatcher(s)
	d.interval = 50 * time.Millisecond

	if err := d.Run(p.ID); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Verify all completed
	for _, name := range []string{"A", "B", "C"} {
		jobs, _ := s.ListJobsByPipeline(p.ID)
		for _, j := range jobs {
			if j.Name == name && j.Status != "completed" {
				t.Errorf("job %s status = %q, want 'completed'", name, j.Status)
			}
		}
	}

	// Verify ordering
	ja, _ := s.GetJob(a.ID)
	jb, _ := s.GetJob(b.ID)
	jc, _ := s.GetJob(c.ID)

	if ja.CompletedAt == nil || jb.StartedAt == nil {
		t.Fatal("timestamps missing")
	}
	if !ja.CompletedAt.Before(*jb.StartedAt) && !ja.CompletedAt.Equal(*jb.StartedAt) {
		t.Error("A should complete before B starts")
	}
	if jb.CompletedAt == nil || jc.StartedAt == nil {
		t.Fatal("timestamps missing")
	}
	if !jb.CompletedAt.Before(*jc.StartedAt) && !jb.CompletedAt.Equal(*jc.StartedAt) {
		t.Error("B should complete before C starts")
	}
}

func TestDispatcherParallelJobs(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")
	a, _ := s.CreateJob(p.ID, "A", "output-A")
	b, _ := s.CreateJob(p.ID, "B", "output-B")
	c, _ := s.CreateJob(p.ID, "C", "output-C")
	dd, _ := s.CreateJob(p.ID, "D", "output-D")

	s.AddDependency(b.ID, a.ID)
	s.AddDependency(c.ID, a.ID)
	s.AddDependency(dd.ID, b.ID)
	s.AddDependency(dd.ID, c.ID)

	s.RegisterAgent("agent-1", "shell", strPtr("cat"), nil, nil, nil)
	s.RegisterAgent("agent-2", "shell", strPtr("cat"), nil, nil, nil)

	d := NewDispatcher(s)
	d.interval = 50 * time.Millisecond

	if err := d.Run(p.ID); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Verify all completed
	jobs, _ := s.ListJobsByPipeline(p.ID)
	for _, j := range jobs {
		if j.Status != "completed" {
			t.Errorf("job %s status = %q, want 'completed'", j.Name, j.Status)
		}
	}

	// A completes first, D completes last
	ja, _ := s.GetJob(a.ID)
	jd, _ := s.GetJob(dd.ID)
	if ja.CompletedAt == nil || jd.StartedAt == nil {
		t.Fatal("timestamps missing")
	}
	if !ja.CompletedAt.Before(*jd.StartedAt) && !ja.CompletedAt.Equal(*jd.StartedAt) {
		t.Error("A should complete before D starts")
	}
}

func TestDispatcherFailedJobHaltsPipeline(t *testing.T) {
	os.Setenv("CLAW_MAX_RETRIES", "0")
	defer os.Unsetenv("CLAW_MAX_RETRIES")

	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")
	a, _ := s.CreateJob(p.ID, "A", "will-fail")
	b, _ := s.CreateJob(p.ID, "B", "never-runs")

	s.AddDependency(b.ID, a.ID)

	s.RegisterAgent("agent-1", "shell", strPtr("sh"), []string{"-c", "exit 1"}, nil, nil)

	d := NewDispatcher(s)
	d.interval = 50 * time.Millisecond

	err := d.Run(p.ID)
	if err == nil {
		t.Fatal("expected pipeline error")
	}

	ja, _ := s.GetJob(a.ID)
	if ja.Status != "failed" {
		t.Errorf("A status = %q, want 'failed'", ja.Status)
	}

	jb, _ := s.GetJob(b.ID)
	if jb.Status != "pending" {
		t.Errorf("B status = %q, want 'pending'", jb.Status)
	}
}

func TestDispatcherRetryPolicy(t *testing.T) {
	os.Setenv("CLAW_MAX_RETRIES", "2")
	defer os.Unsetenv("CLAW_MAX_RETRIES")

	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")
	j, _ := s.CreateJob(p.ID, "retry-test", "will-fail")

	s.RegisterAgent("agent-1", "shell", strPtr("sh"), []string{"-c", "exit 1"}, nil, nil)

	d := NewDispatcher(s)
	d.interval = 50 * time.Millisecond

	err := d.Run(p.ID)
	if err == nil {
		t.Fatal("expected pipeline error")
	}

	got, _ := s.GetJob(j.ID)
	if got.AttemptCount != 3 {
		t.Errorf("attempt_count = %d, want 3 (1 initial + 2 retries)", got.AttemptCount)
	}
	if got.Status != "failed" {
		t.Errorf("status = %q, want 'failed'", got.Status)
	}
}

func TestDispatcherWithGate(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")
	j, _ := s.CreateJob(p.ID, "gated-job", "hello")
	s.SetJobGateCommand(j.ID, "sh -c 'exit 0'")

	s.RegisterAgent("agent-1", "shell", strPtr("cat"), nil, nil, nil)

	d := NewDispatcher(s)
	d.interval = 50 * time.Millisecond

	if err := d.Run(p.ID); err != nil {
		t.Fatalf("Run: %v", err)
	}

	got, _ := s.GetJob(j.ID)
	if got.GateStatus == nil || *got.GateStatus != "passed" {
		t.Errorf("gate_status = %v, want 'passed'", got.GateStatus)
	}
	if got.GateExitCode == nil || *got.GateExitCode != 0 {
		t.Errorf("gate_exit_code = %v, want 0", got.GateExitCode)
	}
	if got.Status != "completed" {
		t.Errorf("status = %q, want 'completed'", got.Status)
	}
}

func TestDispatcherGateFailHaltsPipeline(t *testing.T) {
	os.Setenv("CLAW_MAX_RETRIES", "0")
	defer os.Unsetenv("CLAW_MAX_RETRIES")

	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")
	a, _ := s.CreateJob(p.ID, "A", "hello")
	b, _ := s.CreateJob(p.ID, "B", "world")
	s.AddDependency(b.ID, a.ID)
	s.SetJobGateCommand(a.ID, "sh -c 'exit 1'")

	s.RegisterAgent("agent-1", "shell", strPtr("cat"), nil, nil, nil)

	d := NewDispatcher(s)
	d.interval = 50 * time.Millisecond

	err := d.Run(p.ID)
	if err == nil {
		t.Fatal("expected pipeline error")
	}

	ja, _ := s.GetJob(a.ID)
	if ja.GateStatus == nil || *ja.GateStatus != "failed" {
		t.Errorf("A gate_status = %v, want 'failed'", ja.GateStatus)
	}
	if ja.Status != "failed" {
		t.Errorf("A status = %q, want 'failed'", ja.Status)
	}

	jb, _ := s.GetJob(b.ID)
	if jb.Status != "pending" {
		t.Errorf("B status = %q, want 'pending'", jb.Status)
	}
}

func TestDispatcherContextExtraction(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")
	a, _ := s.CreateJob(p.ID, "phase-1", "build phase 1")
	b, _ := s.CreateJob(p.ID, "phase-2", "build phase 2")
	s.AddDependency(b.ID, a.ID)

	s.RegisterAgent("agent-1", "shell", strPtr("cat"), nil, nil, nil)

	d := NewDispatcher(s)
	d.interval = 50 * time.Millisecond

	if err := d.Run(p.ID); err != nil {
		t.Fatalf("Run: %v", err)
	}

	entries, _ := s.ListContextByPipeline(p.ID)
	if len(entries) < 1 {
		t.Fatal("expected at least 1 context entry")
	}

	found := false
	for _, e := range entries {
		if strings.Contains(e.Content, "phase-1") {
			found = true
			break
		}
	}
	if !found {
		t.Error("no context entry contains 'phase-1'")
	}
}

func TestDispatcherContextInjection(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")
	a, _ := s.CreateJob(p.ID, "phase-1", "build phase 1")
	b, _ := s.CreateJob(p.ID, "phase-2", "build phase 2")
	s.AddDependency(b.ID, a.ID)

	// cat echoes stdin back as output, so B's output should include context
	s.RegisterAgent("agent-1", "shell", strPtr("cat"), nil, nil, nil)

	d := NewDispatcher(s)
	d.interval = 50 * time.Millisecond

	if err := d.Run(p.ID); err != nil {
		t.Fatalf("Run: %v", err)
	}

	jb, _ := s.GetJob(b.ID)
	if jb.Output == nil {
		t.Fatal("B output is nil")
	}
	if !strings.Contains(*jb.Output, "PIPELINE CONTEXT") {
		t.Error("B output missing 'PIPELINE CONTEXT' header")
	}
}

func TestStaleJobRecovery(t *testing.T) {
	os.Setenv("CLAW_MAX_RETRIES", "1")
	defer os.Unsetenv("CLAW_MAX_RETRIES")

	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")
	j, _ := s.CreateJob(p.ID, "stale-job", "pj")
	a, _ := s.RegisterAgent("a1", "shell", nil, nil, nil, nil)

	// Manually simulate a stale assignment
	pastLease := time.Now().Add(-1 * time.Second)
	s.AssignJobToAgent(j.ID, a.ID, pastLease)

	d := NewDispatcher(s)
	d.RecoverStaleJobs(p.ID)

	got, _ := s.GetJob(j.ID)
	if got.Status != "pending" {
		t.Errorf("status = %q, want 'pending' (eligible for retry)", got.Status)
	}

	agent, _ := s.GetAgent(a.ID)
	if agent.Status != "idle" {
		t.Errorf("agent status = %q, want 'idle'", agent.Status)
	}
	if agent.CurrentJobID != nil {
		t.Errorf("agent current_job_id = %v, want nil", agent.CurrentJobID)
	}
}
