package dispatch

import (
	"bytes"
	"strings"
	"testing"
)

func TestManualExecutorBasic(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")
	j, _ := s.CreateJob(p.ID, "phase-1", "Build phase 1")
	a, _ := s.RegisterAgent("human-1", "manual", nil, nil, nil, nil)

	stdin := bytes.NewBufferString("Phase 1 output: all tests pass\n")
	stdout := &bytes.Buffer{}

	err := ExecuteManual(s, j, a, stdin, stdout)
	if err != nil {
		t.Fatalf("ExecuteManual: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "--- JOB:") {
		t.Error("stdout missing '--- JOB:' header")
	}
	if !strings.Contains(out, "Build phase 1") {
		t.Error("stdout missing prompt text")
	}
	if !strings.Contains(out, "Paste agent output") {
		t.Error("stdout missing paste instruction")
	}

	got, _ := s.GetJob(j.ID)
	if got.Output == nil || *got.Output != "Phase 1 output: all tests pass\n" {
		t.Errorf("output = %v, want 'Phase 1 output: all tests pass\\n'", got.Output)
	}
	if got.ExitCode == nil || *got.ExitCode != 0 {
		t.Errorf("exit code = %v, want 0", got.ExitCode)
	}
}

func TestManualExecutorOutputTruncation(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreatePipeline("p", "/tmp")
	j, _ := s.CreateJob(p.ID, "big-job", "do something big")
	a, _ := s.RegisterAgent("human-1", "manual", nil, nil, nil, nil)

	bigInput := strings.Repeat("x", 2_000_000)
	stdin := bytes.NewBufferString(bigInput)
	stdout := &bytes.Buffer{}

	err := ExecuteManual(s, j, a, stdin, stdout)
	if err != nil {
		t.Fatalf("ExecuteManual: %v", err)
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
