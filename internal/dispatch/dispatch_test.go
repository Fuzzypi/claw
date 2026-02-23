package dispatch

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fuzzypi/claw/internal/engram"
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

// writeFakeRTK creates a temp directory with a fake rtk script and returns the dir path.
func writeFakeRTK(t *testing.T, script string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "rtk")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake rtk: %v", err)
	}
	return dir
}

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
	t.Setenv("CLAW_MAX_RETRIES", "0")

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
	t.Setenv("CLAW_MAX_RETRIES", "2")

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
	t.Setenv("CLAW_MAX_RETRIES", "0")

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

func TestFilterThroughRTK_NoRTKOnPath(t *testing.T) {
	t.Setenv("PATH", "/nonexistent")

	input := "hello\nworld"
	output, stats := filterThroughRTK(input)

	if output != input {
		t.Errorf("output = %q, want %q", output, input)
	}
	if stats != "" {
		t.Errorf("stats = %q, want empty string", stats)
	}
}

func TestFilterThroughRTK_RTKFailureFallback(t *testing.T) {
	dir := writeFakeRTK(t, "#!/bin/sh\nexit 1\n")
	t.Setenv("PATH", dir)

	input := "line1\nline2\nline3"
	output, stats := filterThroughRTK(input)

	if output != input {
		t.Errorf("output = %q, want exact passthrough %q", output, input)
	}
	if stats != "" {
		t.Errorf("stats = %q, want empty on failure", stats)
	}
}

func TestExecuteShell_RTKSuccess(t *testing.T) {
	// Fake rtk: reads stdin (discards), writes filtered output to stdout, stats to stderr
	dir := writeFakeRTK(t, "#!/bin/sh\ncat >/dev/null\necho \"FILTERED_OUTPUT\"\necho \"tokens_in=100 tokens_out=50\" >&2\n")
	t.Setenv("PATH", dir+":/usr/bin:/bin")

	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")
	j, _ := s.CreateJob(p.ID, "rtk-test", "raw input")
	a, _ := s.RegisterAgent("a1", "shell", strPtr("cat"), nil, nil, nil)

	err := ExecuteShell(s, j, a)
	if err != nil {
		t.Fatalf("ExecuteShell: %v", err)
	}

	got, _ := s.GetJob(j.ID)
	if got.Output == nil {
		t.Fatal("output is nil")
	}
	if !strings.Contains(*got.Output, "FILTERED_OUTPUT") {
		t.Errorf("stored output = %q, want containing 'FILTERED_OUTPUT'", *got.Output)
	}

	// Verify activity log contains rtk_filtered event
	entries, err := s.ListActivity(&p.ID, 10)
	if err != nil {
		t.Fatalf("ListActivity: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Event == "rtk_filtered" && strings.Contains(e.Detail, "tokens_in=100") {
			found = true
			break
		}
	}
	if !found {
		t.Error("activity_log missing rtk_filtered event with stats")
	}
}

func TestFilterThroughRTK_OrderingBeforeTruncate(t *testing.T) {
	// Fake rtk: discards stdin, emits >1MB to stdout so truncation must fire
	dir := writeFakeRTK(t, "#!/bin/sh\ncat >/dev/null\nhead -c 2000000 /dev/zero | tr '\\0' 'y'\n")
	t.Setenv("PATH", dir+":/usr/bin:/bin")

	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")
	j, _ := s.CreateJob(p.ID, "ordering-test", "small input")
	a, _ := s.RegisterAgent("a1", "shell", strPtr("cat"), nil, nil, nil)

	err := ExecuteShell(s, j, a)
	if err != nil {
		t.Fatalf("ExecuteShell: %v", err)
	}

	got, _ := s.GetJob(j.ID)
	if got.Output == nil {
		t.Fatal("output is nil")
	}
	if !strings.Contains(*got.Output, "[... truncated") {
		t.Error("truncation marker not found — filterThroughRTK must run before truncateOutput")
	}
	if len(*got.Output) > 1_100_000 {
		t.Errorf("output len = %d, want < 1.1MB after truncation", len(*got.Output))
	}
}

func TestStaleJobRecovery(t *testing.T) {
	t.Setenv("CLAW_MAX_RETRIES", "1")

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

// --- Governance Tests ---

func TestExtractPhaseFromName(t *testing.T) {
	tests := []struct {
		name string
		want int
	}{
		{"phase-1-store", 1},
		{"phase-2-dispatch", 2},
		{"phase_3_context", 3},
		{"Phase-10-big", 10},
		{"no-phase-here", -1},
		{"something", -1},
	}
	for _, tt := range tests {
		got := extractPhaseFromName(tt.name)
		if got != tt.want {
			t.Errorf("extractPhaseFromName(%q) = %d, want %d", tt.name, got, tt.want)
		}
	}
}

func TestGovernanceBlocksDispatch(t *testing.T) {
	t.Setenv("CLAW_MAX_RETRIES", "0")

	// Set up project with gates
	tmpDir := t.TempDir()
	setupGovernanceProject(t, tmpDir)

	s := newTestStore(t)
	p, _ := s.CreatePipeline("gov-test", tmpDir)

	// Phase 1 job — completed but gate failed
	phase1 := 1
	j1, _ := s.CreateJobWithPhase(p.ID, "phase-1-store", "build store", &phase1)
	s.SetJobGateCommand(j1.ID, "npm run verify:test:phase01")
	s.UpdateJobStatus(j1.ID, "completed")
	s.SetJobGateResult(j1.ID, "tests failed", 1, "failed")
	s.SetJobCompleted(j1.ID)

	// Phase 2 job — should be blocked by governance
	phase2 := 2
	s.CreateJobWithPhase(p.ID, "phase-2-dispatch", "build dispatch", &phase2)
	s.SetJobGateCommand(int64(2), "npm run verify:test:phase02")

	s.RegisterAgent("a1", "shell", strPtr("cat"), nil, nil, nil)

	d := NewDispatcher(s)
	d.interval = 50 * time.Millisecond

	err := d.Run(p.ID)
	if err == nil {
		t.Fatal("expected pipeline error from governance violation")
	}

	j2, _ := s.GetJob(int64(2))
	if j2.Status != "failed" {
		t.Errorf("phase-2 status = %q, want 'failed'", j2.Status)
	}
	if j2.Output == nil || !strings.Contains(*j2.Output, "governance violation") {
		t.Errorf("phase-2 output = %v, want containing 'governance violation'", j2.Output)
	}

	// Verify governance_violation event in activity log
	entries, _ := s.ListActivity(&p.ID, 50)
	found := false
	for _, e := range entries {
		if e.Event == "governance_violation" {
			found = true
			break
		}
	}
	if !found {
		t.Error("activity log missing 'governance_violation' event")
	}
}

func TestGovernancePasses(t *testing.T) {
	tmpDir := t.TempDir()
	setupGovernanceProject(t, tmpDir)

	s := newTestStore(t)
	p, _ := s.CreatePipeline("gov-pass", tmpDir)

	// Phase 1 job — completed with gate passed
	phase1 := 1
	j1, _ := s.CreateJobWithPhase(p.ID, "phase-1-store", "build store", &phase1)
	s.SetJobGateCommand(j1.ID, "npm run verify:test:phase01")
	s.UpdateJobStatus(j1.ID, "completed")
	s.SetJobGateResult(j1.ID, "all passed", 0, "passed")
	s.SetJobCompleted(j1.ID)

	// Phase 2 job — should pass governance and execute
	phase2 := 2
	j2, _ := s.CreateJobWithPhase(p.ID, "phase-2-dispatch", "echo hello", &phase2)
	s.SetJobGateCommand(j2.ID, "sh -c 'exit 0'")

	s.RegisterAgent("a1", "shell", strPtr("cat"), nil, nil, nil)

	d := NewDispatcher(s)
	d.interval = 50 * time.Millisecond

	if err := d.Run(p.ID); err != nil {
		t.Fatalf("Run: %v", err)
	}

	got, _ := s.GetJob(j2.ID)
	if got.Status != "completed" {
		t.Errorf("phase-2 status = %q, want 'completed'", got.Status)
	}
}

func TestGovernanceAuditToEngram(t *testing.T) {
	tmpDir := t.TempDir()
	setupGovernanceProject(t, tmpDir)

	var sessionStarted, sessionEnded, saveCalled atomic.Int32
	mux := fakeEngramMux(t, &sessionStarted, &sessionEnded, &saveCalled)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	ec := engram.NewClient(engram.Config{URL: ts.URL})
	s := newTestStore(t)
	p, _ := s.CreatePipeline("gov-engram", tmpDir)

	// Phase 1 passed
	phase1 := 1
	j1, _ := s.CreateJobWithPhase(p.ID, "phase-1-store", "build store", &phase1)
	s.SetJobGateCommand(j1.ID, "npm run verify:test:phase01")
	s.UpdateJobStatus(j1.ID, "completed")
	s.SetJobGateResult(j1.ID, "all passed", 0, "passed")
	s.SetJobCompleted(j1.ID)

	// Phase 2 job
	phase2 := 2
	j2, _ := s.CreateJobWithPhase(p.ID, "phase-2-dispatch", "echo hello", &phase2)
	s.SetJobGateCommand(j2.ID, "sh -c 'exit 0'")

	s.RegisterAgent("a1", "shell", strPtr("cat"), nil, nil, nil)

	d := NewDispatcher(s, ec)
	d.interval = 50 * time.Millisecond

	if err := d.Run(p.ID); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Governance decisions + job context saves should have called /api/mem/save
	if saveCalled.Load() < 2 {
		t.Errorf("save calls = %d, want >= 2 (governance + job context)", saveCalled.Load())
	}
}

func setupGovernanceProject(t *testing.T, tmpDir string) {
	t.Helper()
	os.MkdirAll(filepath.Join(tmpDir, ".aos"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "docs", "plan"), 0755)

	os.WriteFile(filepath.Join(tmpDir, ".aos", "GATES.md"), []byte(`| Gate | Script | Phase |
|------|--------|-------|
| store | `+"`verify/test_phase01.verify.cjs`"+` | 1 |
| dispatch | `+"`verify/test_phase02.verify.cjs`"+` | 2 |
`), 0644)

	os.WriteFile(filepath.Join(tmpDir, "docs", "plan", "TEST_PHASE_PLAN.md"), []byte(`## Phase 1 — Store
## Phase 2 — Dispatch
`), 0644)

	os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(`{
  "scripts": {
    "verify:test:phase01": "node verify/test_phase01.verify.cjs",
    "verify:test:phase02": "node verify/test_phase02.verify.cjs"
  }
}`), 0644)
}

// --- Engram Integration Tests ---

func fakeEngramMux(t *testing.T, sessionStarted, sessionEnded, saveCalled *atomic.Int32) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST /api/session/start", func(w http.ResponseWriter, r *http.Request) {
		sessionStarted.Add(1)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"session_id": "test-session-id"})
	})
	mux.HandleFunc("POST /api/session/end", func(w http.ResponseWriter, r *http.Request) {
		sessionEnded.Add(1)
		json.NewEncoder(w).Encode(map[string]any{"ended": true})
	})
	mux.HandleFunc("POST /api/mem/save", func(w http.ResponseWriter, r *http.Request) {
		saveCalled.Add(1)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"id": 1, "created": true})
	})
	mux.HandleFunc("POST /api/mem/search", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{})
	})
	return mux
}

func TestNewDispatcher_WithoutEngram(t *testing.T) {
	s := newTestStore(t)
	d := NewDispatcher(s)
	if d.engram != nil {
		t.Error("engram should be nil when not provided")
	}
}

func TestNewDispatcher_WithEngram(t *testing.T) {
	var ss, se, sc atomic.Int32
	mux := fakeEngramMux(t, &ss, &se, &sc)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	ec := engram.NewClient(engram.Config{URL: ts.URL})
	s := newTestStore(t)
	d := NewDispatcher(s, ec)
	if d.engram == nil {
		t.Error("engram should not be nil when provided")
	}
}

func TestDispatcherSessionLifecycle(t *testing.T) {
	var sessionStarted, sessionEnded, saveCalled atomic.Int32
	mux := fakeEngramMux(t, &sessionStarted, &sessionEnded, &saveCalled)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	ec := engram.NewClient(engram.Config{URL: ts.URL})
	s := newTestStore(t)
	p, _ := s.CreatePipeline("lifecycle-test", "/tmp")
	s.CreateJob(p.ID, "job-1", "hello")
	s.RegisterAgent("a1", "shell", strPtr("cat"), nil, nil, nil)

	d := NewDispatcher(s, ec)
	d.interval = 50 * time.Millisecond

	if err := d.Run(p.ID); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if sessionStarted.Load() != 1 {
		t.Errorf("session/start calls = %d, want 1", sessionStarted.Load())
	}
	if sessionEnded.Load() != 1 {
		t.Errorf("session/end calls = %d, want 1", sessionEnded.Load())
	}
}

func TestDispatcherSaveToEngram(t *testing.T) {
	var sessionStarted, sessionEnded, saveCalled atomic.Int32
	mux := fakeEngramMux(t, &sessionStarted, &sessionEnded, &saveCalled)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	ec := engram.NewClient(engram.Config{URL: ts.URL})
	s := newTestStore(t)
	p, _ := s.CreatePipeline("save-test", "/tmp")
	s.CreateJob(p.ID, "job-1", "hello")
	s.RegisterAgent("a1", "shell", strPtr("cat"), nil, nil, nil)

	d := NewDispatcher(s, ec)
	d.interval = 50 * time.Millisecond

	if err := d.Run(p.ID); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if saveCalled.Load() < 1 {
		t.Errorf("save calls = %d, want >= 1", saveCalled.Load())
	}
}

func TestDispatcherEngramUnavailable(t *testing.T) {
	// Server returns 500 on health check — client becomes unavailable
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	ec := engram.NewClient(engram.Config{URL: ts.URL})
	s := newTestStore(t)
	p, _ := s.CreatePipeline("unavail-test", "/tmp")
	s.CreateJob(p.ID, "job-1", "hello")
	s.RegisterAgent("a1", "shell", strPtr("cat"), nil, nil, nil)

	d := NewDispatcher(s, ec)
	d.interval = 50 * time.Millisecond

	// Pipeline should complete normally despite Engram being unavailable
	if err := d.Run(p.ID); err != nil {
		t.Fatalf("Run should succeed with unavailable Engram: %v", err)
	}

	got, _ := s.GetJob(int64(1))
	if got.Status != "completed" {
		t.Errorf("job status = %q, want 'completed'", got.Status)
	}
}

func TestBuildFullPromptWithEngram_MemoryInjection(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST /api/mem/search", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]engram.SearchResult{
			{ID: 1, Title: "Prior Job Result", Type: "discovery", Snippet: "important context from last run"},
		})
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	ec := engram.NewClient(engram.Config{URL: ts.URL})

	s := newTestStore(t)
	p, _ := s.CreatePipeline("mem-test", "/tmp")
	j, _ := s.CreateJob(p.ID, "phase-1", "build the thing")

	result := buildFullPromptWithEngram(s, j, ec)

	if !strings.Contains(result, "ENGRAM MEMORY") {
		t.Error("prompt missing 'ENGRAM MEMORY' section")
	}
	if !strings.Contains(result, "Prior Job Result") {
		t.Error("prompt missing search result title")
	}
	if !strings.Contains(result, "important context from last run") {
		t.Error("prompt missing search result snippet")
	}
	if !strings.Contains(result, "build the thing") {
		t.Error("prompt missing original job prompt")
	}
}

func TestBuildFullPromptWithEngram_NilClient(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreatePipeline("nil-test", "/tmp")
	j, _ := s.CreateJob(p.ID, "job-1", "hello world")

	withEngram := buildFullPromptWithEngram(s, j, nil)
	withoutEngram := buildFullPrompt(s, j)

	if withEngram != withoutEngram {
		t.Errorf("nil client prompt mismatch:\n  with:    %q\n  without: %q", withEngram, withoutEngram)
	}
}

func TestBuildFullPromptWithEngram_UnavailableClient(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	ec := engram.NewClient(engram.Config{URL: ts.URL})

	s := newTestStore(t)
	p, _ := s.CreatePipeline("unavail-test", "/tmp")
	j, _ := s.CreateJob(p.ID, "job-1", "hello world")

	result := buildFullPromptWithEngram(s, j, ec)
	baseline := buildFullPrompt(s, j)

	if result != baseline {
		t.Errorf("unavailable client prompt mismatch:\n  got:  %q\n  want: %q", result, baseline)
	}
}
