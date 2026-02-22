package store

import (
	"strings"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestCreateAndGetPipeline(t *testing.T) {
	s := newTestStore(t)
	p, err := s.CreatePipeline("test-pipeline", "/tmp/project")
	if err != nil {
		t.Fatalf("CreatePipeline: %v", err)
	}
	got, err := s.GetPipeline(p.ID)
	if err != nil {
		t.Fatalf("GetPipeline: %v", err)
	}
	if got.Name != "test-pipeline" {
		t.Errorf("Name = %q, want %q", got.Name, "test-pipeline")
	}
	if got.ProjectDir != "/tmp/project" {
		t.Errorf("ProjectDir = %q, want %q", got.ProjectDir, "/tmp/project")
	}
	if got.Status != "active" {
		t.Errorf("Status = %q, want %q", got.Status, "active")
	}
}

func TestListPipelines(t *testing.T) {
	s := newTestStore(t)
	s.CreatePipeline("p1", "/tmp/a")
	s.CreatePipeline("p2", "/tmp/b")
	list, err := s.ListPipelines()
	if err != nil {
		t.Fatalf("ListPipelines: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len = %d, want 2", len(list))
	}
}

func TestUpdatePipelineStatus(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")
	if err := s.UpdatePipelineStatus(p.ID, "completed"); err != nil {
		t.Fatalf("UpdatePipelineStatus: %v", err)
	}
	got, _ := s.GetPipeline(p.ID)
	if got.Status != "completed" {
		t.Errorf("Status = %q, want %q", got.Status, "completed")
	}
}

func TestCreateAndGetJob(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")
	j, err := s.CreateJob(p.ID, "build", "run make build")
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	got, err := s.GetJob(j.ID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if got.Name != "build" {
		t.Errorf("Name = %q, want %q", got.Name, "build")
	}
	if got.Prompt != "run make build" {
		t.Errorf("Prompt = %q, want %q", got.Prompt, "run make build")
	}
	if got.Status != "pending" {
		t.Errorf("Status = %q, want %q", got.Status, "pending")
	}
	if got.PipelineID != p.ID {
		t.Errorf("PipelineID = %d, want %d", got.PipelineID, p.ID)
	}
}

func TestListJobsByPipeline(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")
	s.CreateJob(p.ID, "a", "pa")
	s.CreateJob(p.ID, "b", "pb")
	s.CreateJob(p.ID, "c", "pc")
	list, err := s.ListJobsByPipeline(p.ID)
	if err != nil {
		t.Fatalf("ListJobsByPipeline: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("len = %d, want 3", len(list))
	}
}

func TestUpdateJobStatus(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")
	j, _ := s.CreateJob(p.ID, "j", "pj")
	if err := s.UpdateJobStatus(j.ID, "running"); err != nil {
		t.Fatalf("UpdateJobStatus: %v", err)
	}
	got, _ := s.GetJob(j.ID)
	if got.Status != "running" {
		t.Errorf("Status = %q, want %q", got.Status, "running")
	}
}

func TestSetJobOutput(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")
	j, _ := s.CreateJob(p.ID, "j", "pj")
	if err := s.SetJobOutput(j.ID, "hello world", 0); err != nil {
		t.Fatalf("SetJobOutput: %v", err)
	}
	got, _ := s.GetJob(j.ID)
	if got.Output == nil || *got.Output != "hello world" {
		t.Errorf("Output = %v, want %q", got.Output, "hello world")
	}
	if got.ExitCode == nil || *got.ExitCode != 0 {
		t.Errorf("ExitCode = %v, want 0", got.ExitCode)
	}
}

func TestSetJobOutputTruncation(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")
	j, _ := s.CreateJob(p.ID, "j", "pj")

	// Build a string > 1MB
	big := strings.Repeat("A", 512*1024) + strings.Repeat("B", 512*1024) // 1MB exactly won't trigger, make it bigger
	big += strings.Repeat("C", 1024)                                      // now > 1MB

	if err := s.SetJobOutput(j.ID, big, 1); err != nil {
		t.Fatalf("SetJobOutput: %v", err)
	}
	got, _ := s.GetJob(j.ID)
	if got.Output == nil {
		t.Fatal("Output is nil")
	}
	out := *got.Output

	// Must be smaller than original
	if len(out) >= len(big) {
		t.Errorf("output not truncated: len=%d, original=%d", len(out), len(big))
	}

	// First 256KB should match original
	if out[:keepBytes] != big[:keepBytes] {
		t.Error("first 256KB does not match original")
	}

	// Last 256KB should match original
	if out[len(out)-keepBytes:] != big[len(big)-keepBytes:] {
		t.Error("last 256KB does not match original")
	}

	// Marker in the middle
	if !strings.Contains(out, "[... truncated") {
		t.Error("truncation marker not found")
	}
}

func TestRegisterAndGetAgent(t *testing.T) {
	s := newTestStore(t)
	cmd := "claude"
	cwd := "/tmp/project"
	timeout := 300
	a, err := s.RegisterAgent("builder-1", "shell", &cmd, []string{"--prompt-file", "-"}, &cwd, &timeout)
	if err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}
	got, err := s.GetAgent(a.ID)
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if got.Name != "builder-1" {
		t.Errorf("Name = %q, want %q", got.Name, "builder-1")
	}
	if got.Type != "shell" {
		t.Errorf("Type = %q, want %q", got.Type, "shell")
	}
	if got.Command == nil || *got.Command != "claude" {
		t.Errorf("Command = %v, want %q", got.Command, "claude")
	}
	if len(got.Args) != 2 || got.Args[0] != "--prompt-file" || got.Args[1] != "-" {
		t.Errorf("Args = %v, want [--prompt-file -]", got.Args)
	}
	if got.Cwd == nil || *got.Cwd != "/tmp/project" {
		t.Errorf("Cwd = %v, want %q", got.Cwd, "/tmp/project")
	}
	if got.TimeoutSecs != 300 {
		t.Errorf("TimeoutSecs = %d, want 300", got.TimeoutSecs)
	}
	if got.Status != "idle" {
		t.Errorf("Status = %q, want %q", got.Status, "idle")
	}
}

func TestListAgents(t *testing.T) {
	s := newTestStore(t)
	s.RegisterAgent("a1", "shell", nil, nil, nil, nil)
	s.RegisterAgent("a2", "manual", nil, nil, nil, nil)
	list, err := s.ListAgents()
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len = %d, want 2", len(list))
	}
}

func TestUpdateAgentStatus(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")
	j, _ := s.CreateJob(p.ID, "j", "pj")
	a, _ := s.RegisterAgent("a1", "shell", nil, nil, nil, nil)

	// Set to busy with job
	jobID := j.ID
	if err := s.UpdateAgentStatus(a.ID, "busy", &jobID); err != nil {
		t.Fatalf("UpdateAgentStatus (busy): %v", err)
	}
	got, _ := s.GetAgent(a.ID)
	if got.Status != "busy" {
		t.Errorf("Status = %q, want %q", got.Status, "busy")
	}
	if got.CurrentJobID == nil || *got.CurrentJobID != j.ID {
		t.Errorf("CurrentJobID = %v, want %d", got.CurrentJobID, j.ID)
	}

	// Set back to idle with nil job
	if err := s.UpdateAgentStatus(a.ID, "idle", nil); err != nil {
		t.Fatalf("UpdateAgentStatus (idle): %v", err)
	}
	got, _ = s.GetAgent(a.ID)
	if got.Status != "idle" {
		t.Errorf("Status = %q, want %q", got.Status, "idle")
	}
	if got.CurrentJobID != nil {
		t.Errorf("CurrentJobID = %v, want nil", got.CurrentJobID)
	}
}

func TestAgentUniqueNameConstraint(t *testing.T) {
	s := newTestStore(t)
	_, err := s.RegisterAgent("builder-1", "shell", nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("first RegisterAgent: %v", err)
	}
	_, err = s.RegisterAgent("builder-1", "shell", nil, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for duplicate agent name, got nil")
	}
}

func TestAddContextEntry(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")
	j, _ := s.CreateJob(p.ID, "j", "pj")
	e, err := s.AddContextEntry(p.ID, j.ID, "some context data")
	if err != nil {
		t.Fatalf("AddContextEntry: %v", err)
	}
	if e.Content != "some context data" {
		t.Errorf("Content = %q, want %q", e.Content, "some context data")
	}

	list, err := s.ListContextByPipeline(p.ID)
	if err != nil {
		t.Fatalf("ListContextByPipeline: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("len = %d, want 1", len(list))
	}
	if list[0].Content != "some context data" {
		t.Errorf("Content = %q, want %q", list[0].Content, "some context data")
	}
}

func TestAddDependency(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")
	a, _ := s.CreateJob(p.ID, "A", "pa")
	b, _ := s.CreateJob(p.ID, "B", "pb")
	c, _ := s.CreateJob(p.ID, "C", "pc")

	// B depends on A
	if err := s.AddDependency(b.ID, a.ID); err != nil {
		t.Fatalf("AddDependency B->A: %v", err)
	}
	// C depends on B
	if err := s.AddDependency(c.ID, b.ID); err != nil {
		t.Fatalf("AddDependency C->B: %v", err)
	}
}

func TestListReadyJobs(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")
	a, _ := s.CreateJob(p.ID, "A", "pa")
	b, _ := s.CreateJob(p.ID, "B", "pb")
	c, _ := s.CreateJob(p.ID, "C", "pc")

	s.AddDependency(b.ID, a.ID) // B depends on A
	s.AddDependency(c.ID, b.ID) // C depends on B

	// Initially only A is ready
	ready, _ := s.ListReadyJobs(p.ID)
	if len(ready) != 1 || ready[0].ID != a.ID {
		t.Fatalf("ready = %v, want [A]", jobNames(ready))
	}

	// Complete A -> B is ready
	s.UpdateJobStatus(a.ID, "completed")
	ready, _ = s.ListReadyJobs(p.ID)
	if len(ready) != 1 || ready[0].ID != b.ID {
		t.Fatalf("ready = %v, want [B]", jobNames(ready))
	}

	// Complete B -> C is ready
	s.UpdateJobStatus(b.ID, "completed")
	ready, _ = s.ListReadyJobs(p.ID)
	if len(ready) != 1 || ready[0].ID != c.ID {
		t.Fatalf("ready = %v, want [C]", jobNames(ready))
	}
}

func TestListReadyJobsParallel(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")
	a, _ := s.CreateJob(p.ID, "A", "pa")
	b, _ := s.CreateJob(p.ID, "B", "pb")
	c, _ := s.CreateJob(p.ID, "C", "pc")
	d, _ := s.CreateJob(p.ID, "D", "pd")

	s.AddDependency(b.ID, a.ID) // B depends on A
	s.AddDependency(c.ID, a.ID) // C depends on A
	s.AddDependency(d.ID, b.ID) // D depends on B
	s.AddDependency(d.ID, c.ID) // D depends on C

	// Initially only A
	ready, _ := s.ListReadyJobs(p.ID)
	if len(ready) != 1 || ready[0].ID != a.ID {
		t.Fatalf("ready = %v, want [A]", jobNames(ready))
	}

	// Complete A -> B and C are ready (parallel)
	s.UpdateJobStatus(a.ID, "completed")
	ready, _ = s.ListReadyJobs(p.ID)
	if len(ready) != 2 {
		t.Fatalf("ready count = %d, want 2", len(ready))
	}
	if ready[0].ID != b.ID || ready[1].ID != c.ID {
		t.Fatalf("ready = %v, want [B, C]", jobNames(ready))
	}

	// Complete B -> still only C (D needs both)
	s.UpdateJobStatus(b.ID, "completed")
	ready, _ = s.ListReadyJobs(p.ID)
	if len(ready) != 1 || ready[0].ID != c.ID {
		t.Fatalf("ready = %v, want [C]", jobNames(ready))
	}

	// Complete C -> D is ready
	s.UpdateJobStatus(c.ID, "completed")
	ready, _ = s.ListReadyJobs(p.ID)
	if len(ready) != 1 || ready[0].ID != d.ID {
		t.Fatalf("ready = %v, want [D]", jobNames(ready))
	}
}

func TestCycleDetection(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")
	a, _ := s.CreateJob(p.ID, "A", "pa")
	b, _ := s.CreateJob(p.ID, "B", "pb")
	c, _ := s.CreateJob(p.ID, "C", "pc")

	if err := s.AddDependency(a.ID, b.ID); err != nil {
		t.Fatalf("A->B: %v", err)
	}
	if err := s.AddDependency(b.ID, c.ID); err != nil {
		t.Fatalf("B->C: %v", err)
	}
	// C depends on A would create cycle: A->B->C->A
	err := s.AddDependency(c.ID, a.ID)
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error = %q, want cycle error", err.Error())
	}
}

func TestCycleDetectionSelfReference(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")
	a, _ := s.CreateJob(p.ID, "A", "pa")

	err := s.AddDependency(a.ID, a.ID)
	if err == nil {
		t.Fatal("expected self-reference error, got nil")
	}
}

func jobNames(jobs []*Job) []string {
	names := make([]string, len(jobs))
	for i, j := range jobs {
		names[i] = j.Name
	}
	return names
}

func TestLogActivity(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")

	s.LogActivity(&p.ID, nil, nil, "pipeline_started", "Pipeline started")
	s.LogActivity(&p.ID, nil, nil, "job_dispatched", "Job dispatched")
	s.LogActivity(&p.ID, nil, nil, "job_completed", "Job completed")

	entries, err := s.ListActivity(nil, 50)
	if err != nil {
		t.Fatalf("ListActivity: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("entry count = %d, want 3", len(entries))
	}
	// Newest first
	if entries[0].Event != "job_completed" {
		t.Errorf("entries[0].Event = %q, want 'job_completed'", entries[0].Event)
	}
	if entries[2].Event != "pipeline_started" {
		t.Errorf("entries[2].Event = %q, want 'pipeline_started'", entries[2].Event)
	}
}

func TestLogActivityFilterByPipeline(t *testing.T) {
	s := newTestStore(t)
	p1, _ := s.CreatePipeline("p1", "/tmp")
	p2, _ := s.CreatePipeline("p2", "/tmp")

	s.LogActivity(&p1.ID, nil, nil, "event1", "detail1")
	s.LogActivity(&p1.ID, nil, nil, "event2", "detail2")
	s.LogActivity(&p2.ID, nil, nil, "event3", "detail3")

	entries1, _ := s.ListActivity(&p1.ID, 50)
	if len(entries1) != 2 {
		t.Fatalf("pipeline 1 entries = %d, want 2", len(entries1))
	}

	entries2, _ := s.ListActivity(&p2.ID, 50)
	if len(entries2) != 1 {
		t.Fatalf("pipeline 2 entries = %d, want 1", len(entries2))
	}
}

func TestLogActivityLimit(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")

	for i := 0; i < 10; i++ {
		s.LogActivity(&p.ID, nil, nil, "event", "detail")
	}

	entries, _ := s.ListActivity(nil, 3)
	if len(entries) != 3 {
		t.Fatalf("entry count = %d, want 3", len(entries))
	}
}
