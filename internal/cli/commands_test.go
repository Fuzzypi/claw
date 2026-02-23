package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func TestPipelineCreateCmd(t *testing.T) {
	s := newTestStore(t)
	cmd := PipelineCreateCmd(s)
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetArgs([]string{"test-pipeline", "--project", "/tmp/test"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	pipelines, err := s.ListPipelines()
	if err != nil {
		t.Fatalf("ListPipelines: %v", err)
	}
	if len(pipelines) != 1 {
		t.Fatalf("pipeline count = %d, want 1", len(pipelines))
	}
	if pipelines[0].Name != "test-pipeline" {
		t.Errorf("name = %q, want 'test-pipeline'", pipelines[0].Name)
	}
	if !strings.Contains(out.String(), "Created pipeline") {
		t.Errorf("output = %q, want containing 'Created pipeline'", out.String())
	}
}

func TestAgentRegisterCmd(t *testing.T) {
	s := newTestStore(t)
	cmd := AgentRegisterCmd(s)
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetArgs([]string{"builder-1", "--type", "shell", "--command", "cat"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	agents, err := s.ListAgents()
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("agent count = %d, want 1", len(agents))
	}
	if agents[0].Name != "builder-1" {
		t.Errorf("name = %q, want 'builder-1'", agents[0].Name)
	}
	if agents[0].Type != "shell" {
		t.Errorf("type = %q, want 'shell'", agents[0].Type)
	}
}

func TestAgentRegisterManualType(t *testing.T) {
	s := newTestStore(t)
	cmd := AgentRegisterCmd(s)
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetArgs([]string{"human-1", "--type", "manual"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	agents, _ := s.ListAgents()
	if len(agents) != 1 {
		t.Fatalf("agent count = %d, want 1", len(agents))
	}
	if agents[0].Type != "manual" {
		t.Errorf("type = %q, want 'manual'", agents[0].Type)
	}
	if agents[0].Command != nil {
		t.Errorf("command = %v, want nil", agents[0].Command)
	}
}

func TestAgentRegisterShellRequiresCommand(t *testing.T) {
	s := newTestStore(t)
	cmd := AgentRegisterCmd(s)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"bad-agent", "--type", "shell"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for shell agent without --command")
	}
}

func TestAgentListCmd(t *testing.T) {
	s := newTestStore(t)
	cmd1 := "cat"
	s.RegisterAgent("agent-a", "shell", &cmd1, nil, nil, nil)
	s.RegisterAgent("agent-b", "manual", nil, nil, nil, nil)

	cmd := AgentListCmd(s)
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "agent-a") {
		t.Error("output missing 'agent-a'")
	}
	if !strings.Contains(output, "agent-b") {
		t.Error("output missing 'agent-b'")
	}
}

func TestJobAddCmd(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")

	tmpDir := t.TempDir()
	promptFile := filepath.Join(tmpDir, "prompt.txt")
	os.WriteFile(promptFile, []byte("Build the thing"), 0644)

	cmd := JobAddCmd(s)
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetArgs([]string{"1", "phase-1", "--prompt-file", promptFile})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	jobs, _ := s.ListJobsByPipeline(p.ID)
	if len(jobs) != 1 {
		t.Fatalf("job count = %d, want 1", len(jobs))
	}
	if jobs[0].Name != "phase-1" {
		t.Errorf("name = %q, want 'phase-1'", jobs[0].Name)
	}
	if jobs[0].Prompt != "Build the thing" {
		t.Errorf("prompt = %q, want 'Build the thing'", jobs[0].Prompt)
	}
}

func TestJobAddWithPhase(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")

	tmpDir := t.TempDir()
	promptFile := filepath.Join(tmpDir, "prompt.txt")
	os.WriteFile(promptFile, []byte("Build phase 3"), 0644)

	cmd := JobAddCmd(s)
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetArgs([]string{"1", "phase-3-context", "--prompt-file", promptFile, "--phase", "3"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	jobs, _ := s.ListJobsByPipeline(p.ID)
	if len(jobs) != 1 {
		t.Fatalf("job count = %d, want 1", len(jobs))
	}
	if jobs[0].PhaseNumber == nil || *jobs[0].PhaseNumber != 3 {
		t.Errorf("PhaseNumber = %v, want 3", jobs[0].PhaseNumber)
	}
}

func TestJobAddWithoutPhase(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")

	tmpDir := t.TempDir()
	promptFile := filepath.Join(tmpDir, "prompt.txt")
	os.WriteFile(promptFile, []byte("No phase"), 0644)

	cmd := JobAddCmd(s)
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetArgs([]string{"1", "no-phase-job", "--prompt-file", promptFile})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	jobs, _ := s.ListJobsByPipeline(p.ID)
	if len(jobs) != 1 {
		t.Fatalf("job count = %d, want 1", len(jobs))
	}
	if jobs[0].PhaseNumber != nil {
		t.Errorf("PhaseNumber = %v, want nil", jobs[0].PhaseNumber)
	}
}

func TestJobAddWithDependency(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")
	s.CreateJob(p.ID, "phase-1", "first step")

	tmpDir := t.TempDir()
	promptFile := filepath.Join(tmpDir, "prompt.txt")
	os.WriteFile(promptFile, []byte("Second step"), 0644)

	cmd := JobAddCmd(s)
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetArgs([]string{"1", "phase-2", "--prompt-file", promptFile, "--depends-on", "phase-1"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// phase-2 should not be ready since phase-1 is pending
	ready, _ := s.ListReadyJobs(p.ID)
	readyNames := make([]string, len(ready))
	for i, j := range ready {
		readyNames[i] = j.Name
	}
	for _, name := range readyNames {
		if name == "phase-2" {
			t.Error("phase-2 should not be ready while phase-1 is pending")
		}
	}
}

func TestOutputCmd(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")
	j, _ := s.CreateJob(p.ID, "j", "pj")
	s.SetJobOutput(j.ID, "test output here", 0)

	cmd := OutputCmd(s)
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetArgs([]string{"1"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !strings.Contains(out.String(), "test output here") {
		t.Errorf("output = %q, want containing 'test output here'", out.String())
	}
}

func TestOutputCmdNoOutput(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")
	s.CreateJob(p.ID, "j", "pj")

	cmd := OutputCmd(s)
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetArgs([]string{"1"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !strings.Contains(out.String(), "No output captured") {
		t.Errorf("output = %q, want containing 'No output captured'", out.String())
	}
}

func TestGetJobByName(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")
	s.CreateJob(p.ID, "alpha", "pa")
	s.CreateJob(p.ID, "beta", "pb")

	got, err := s.GetJobByName(p.ID, "alpha")
	if err != nil {
		t.Fatalf("GetJobByName(alpha): %v", err)
	}
	if got.Name != "alpha" {
		t.Errorf("name = %q, want 'alpha'", got.Name)
	}

	_, err = s.GetJobByName(p.ID, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent job name")
	}
}

func TestMixedPipelineShellAndManual(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")
	a, _ := s.CreateJob(p.ID, "A", "shell task")
	b, _ := s.CreateJob(p.ID, "B", "manual task")

	s.AddDependency(b.ID, a.ID)

	cmd := "cat"
	s.RegisterAgent("shell-1", "shell", &cmd, nil, nil, nil)
	s.RegisterAgent("human-1", "manual", nil, nil, nil, nil)

	// Only A should be ready
	ready, _ := s.ListReadyJobs(p.ID)
	if len(ready) != 1 || ready[0].Name != "A" {
		t.Fatalf("ready = %v, want [A]", jobNames(ready))
	}

	// Complete A, now B should be ready
	s.UpdateJobStatus(a.ID, "completed")
	ready, _ = s.ListReadyJobs(p.ID)
	if len(ready) != 1 || ready[0].Name != "B" {
		t.Fatalf("ready = %v, want [B]", jobNames(ready))
	}
}

func TestSummaryCmd(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreatePipeline("my-pipeline", "/tmp")
	j, _ := s.CreateJob(p.ID, "j", "pj")
	s.AddContextEntry(p.ID, j.ID, "Job \"j\": PASSED — 5 tests passed")

	cmd := SummaryCmd(s)
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetArgs([]string{"1"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Handoff Summary") {
		t.Error("output missing 'Handoff Summary'")
	}
	if !strings.Contains(output, "5 tests passed") {
		t.Error("output missing context entry content")
	}
}

func TestContextCmd(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")
	j, _ := s.CreateJob(p.ID, "j", "pj")
	s.AddContextEntry(p.ID, j.ID, "some context data")

	cmd := ContextCmd(s)
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetArgs([]string{"1"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !strings.Contains(out.String(), "some context data") {
		t.Error("output missing context entry content")
	}
}

func TestGateCmd(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")
	j, _ := s.CreateJob(p.ID, "j", "pj")
	s.SetJobGateResult(j.ID, "all checks passed", 0, "passed")

	cmd := GateCmd(s)
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetArgs([]string{"1"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "passed") {
		t.Error("output missing gate status")
	}
	if !strings.Contains(output, "0") {
		t.Error("output missing exit code")
	}
}

func TestGateCmdNoGate(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")
	s.CreateJob(p.ID, "j", "pj")

	cmd := GateCmd(s)
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetArgs([]string{"1"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !strings.Contains(out.String(), "No gate configured") {
		t.Errorf("output = %q, want containing 'No gate configured'", out.String())
	}
}

func jobNames(jobs []*store.Job) []string {
	names := make([]string, len(jobs))
	for i, j := range jobs {
		names[i] = j.Name
	}
	return names
}

func TestInitCmd(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, ".aos"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "docs", "plan"), 0755)

	os.WriteFile(filepath.Join(tmpDir, ".aos", "GATES.md"), []byte(`| Gate | Script | Phase |
|------|--------|-------|
| store | `+"`verify/test_phase01.verify.cjs`"+` | 1 |
| dispatch | `+"`verify/test_phase02.verify.cjs`"+` | 2 |
`), 0644)

	os.WriteFile(filepath.Join(tmpDir, "docs", "plan", "TEST_PHASE_PLAN.md"), []byte(`## Phase 0 — Foundation _(complete)_

## Phase 1 — Store + Job Model

## Phase 2 — Dispatch Engine
`), 0644)

	os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(`{
  "scripts": {
    "verify:test:phase01": "node verify/test_phase01.verify.cjs",
    "verify:test:phase02": "node verify/test_phase02.verify.cjs"
  }
}`), 0644)

	s := newTestStore(t)
	cmd := InitCmd(s)
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetArgs([]string{tmpDir})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// Verify pipeline created
	pipelines, _ := s.ListPipelines()
	if len(pipelines) != 1 {
		t.Fatalf("pipeline count = %d, want 1", len(pipelines))
	}
	if pipelines[0].Name != filepath.Base(tmpDir) {
		t.Errorf("pipeline name = %q, want %q", pipelines[0].Name, filepath.Base(tmpDir))
	}

	// Verify jobs (phase 0 skipped, phases 1 and 2 created)
	jobs, _ := s.ListJobsByPipeline(pipelines[0].ID)
	if len(jobs) != 2 {
		t.Fatalf("job count = %d, want 2", len(jobs))
	}
	if !strings.Contains(jobs[0].Name, "phase-1-") {
		t.Errorf("job[0] name = %q, want containing 'phase-1-'", jobs[0].Name)
	}
	if !strings.Contains(jobs[1].Name, "phase-2-") {
		t.Errorf("job[1] name = %q, want containing 'phase-2-'", jobs[1].Name)
	}

	// Verify gates set
	if jobs[0].GateCommand == nil || !strings.Contains(*jobs[0].GateCommand, "verify:test:phase01") {
		t.Errorf("job[0] gate = %v, want containing 'verify:test:phase01'", jobs[0].GateCommand)
	}
	if jobs[1].GateCommand == nil || !strings.Contains(*jobs[1].GateCommand, "verify:test:phase02") {
		t.Errorf("job[1] gate = %v, want containing 'verify:test:phase02'", jobs[1].GateCommand)
	}

	// Verify dependency: job 2 depends on job 1
	ready, _ := s.ListReadyJobs(pipelines[0].ID)
	readyNames := jobNames(ready)
	if len(readyNames) != 1 || !strings.Contains(readyNames[0], "phase-1-") {
		t.Errorf("ready jobs = %v, want only phase-1", readyNames)
	}

	// Verify output
	output := out.String()
	if !strings.Contains(output, "Initialized pipeline") {
		t.Error("output missing 'Initialized pipeline'")
	}
}

func TestInitCmdNoPhases(t *testing.T) {
	tmpDir := t.TempDir()
	s := newTestStore(t)
	cmd := InitCmd(s)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{tmpDir})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for project with no phases")
	}
	if !strings.Contains(err.Error(), "no phases") {
		t.Errorf("error = %q, want containing 'no phases'", err.Error())
	}
}

func TestInitCmdNoGates(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "docs", "plan"), 0755)

	os.WriteFile(filepath.Join(tmpDir, "docs", "plan", "TEST_PHASE_PLAN.md"), []byte(`## Phase 1 — Store + Job Model

## Phase 2 — Dispatch Engine
`), 0644)

	s := newTestStore(t)
	cmd := InitCmd(s)
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetArgs([]string{tmpDir})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	pipelines, _ := s.ListPipelines()
	if len(pipelines) != 1 {
		t.Fatalf("pipeline count = %d, want 1", len(pipelines))
	}

	jobs, _ := s.ListJobsByPipeline(pipelines[0].ID)
	if len(jobs) != 2 {
		t.Fatalf("job count = %d, want 2", len(jobs))
	}

	// No gates should be set
	for _, j := range jobs {
		if j.GateCommand != nil {
			t.Errorf("job %q has gate %q, want nil", j.Name, *j.GateCommand)
		}
	}

	if !strings.Contains(out.String(), "With gates: 0") {
		t.Error("output missing 'With gates: 0'")
	}
}

func TestLogCmd(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")
	s.LogActivity(&p.ID, nil, nil, "pipeline_started", "Pipeline started")
	s.LogActivity(&p.ID, nil, nil, "job_dispatched", "Job dispatched to builder")

	cmd := LogCmd(s)
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Pipeline started") {
		t.Error("output missing 'Pipeline started'")
	}
	if !strings.Contains(output, "Job dispatched to builder") {
		t.Error("output missing 'Job dispatched to builder'")
	}
}

func TestLogCmdEmpty(t *testing.T) {
	s := newTestStore(t)
	cmd := LogCmd(s)
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !strings.Contains(out.String(), "No activity recorded") {
		t.Errorf("output = %q, want containing 'No activity recorded'", out.String())
	}
}

func TestLogCmdWithPipelineFilter(t *testing.T) {
	s := newTestStore(t)
	p1, _ := s.CreatePipeline("p1", "/tmp")
	p2, _ := s.CreatePipeline("p2", "/tmp")

	s.LogActivity(&p1.ID, nil, nil, "event", "Pipeline 1 event")
	s.LogActivity(&p2.ID, nil, nil, "event", "Pipeline 2 event")

	cmd := LogCmd(s)
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetArgs([]string{"--pipeline", "1"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Pipeline 1 event") {
		t.Error("output missing 'Pipeline 1 event'")
	}
	if strings.Contains(output, "Pipeline 2 event") {
		t.Error("output should not contain 'Pipeline 2 event'")
	}
}

func TestAllCommandsHaveHelp(t *testing.T) {
	s := newTestStore(t)

	commands := map[string]*store.Store{
		"RunCmd":     s,
		"StatusCmd":  s,
		"OutputCmd":  s,
		"SummaryCmd": s,
		"ContextCmd": s,
		"GateCmd":    s,
		"InitCmd":    s,
		"LogCmd":     s,
	}

	cmdFuncs := []struct {
		name string
		long string
	}{
		{"RunCmd", RunCmd(s).Long},
		{"StatusCmd", StatusCmd(s).Long},
		{"OutputCmd", OutputCmd(s).Long},
		{"SummaryCmd", SummaryCmd(s).Long},
		{"ContextCmd", ContextCmd(s).Long},
		{"GateCmd", GateCmd(s).Long},
		{"InitCmd", InitCmd(s).Long},
		{"LogCmd", LogCmd(s).Long},
		{"PipelineCreateCmd", PipelineCreateCmd(s).Long},
		{"AgentRegisterCmd", AgentRegisterCmd(s).Long},
		{"AgentListCmd", AgentListCmd(s).Long},
		{"JobAddCmd", JobAddCmd(s).Long},
	}

	_ = commands // used above

	for _, cf := range cmdFuncs {
		if cf.long == "" {
			t.Errorf("%s has empty Long description", cf.name)
		}
	}
}
