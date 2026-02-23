package aos

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadLaws(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "governance", "laws"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "governance", "laws", "LAWSET.md"), []byte(`# Laws

### Law 1: Phase Lock
No phase N+1 work begins until phase N gates pass.

### Law 2: Gate Integrity
Every gated phase must have a gate command configured.

## System Laws

| ID | Rule |
|---|---|
| LAW-GL-001 | All changes require tests |
| LAW-GL-002 | No secrets in source |
`), 0644)

	laws, err := ReadLaws(tmpDir)
	if err != nil {
		t.Fatalf("ReadLaws: %v", err)
	}
	if len(laws) != 4 {
		t.Fatalf("law count = %d, want 4", len(laws))
	}
	if laws[0].ID != "LAW-1" {
		t.Errorf("laws[0].ID = %q, want 'LAW-1'", laws[0].ID)
	}
	if laws[0].Name != "Phase Lock" {
		t.Errorf("laws[0].Name = %q, want 'Phase Lock'", laws[0].Name)
	}
	if laws[0].Rule != "No phase N+1 work begins until phase N gates pass." {
		t.Errorf("laws[0].Rule = %q", laws[0].Rule)
	}
	if laws[0].Category != "core" {
		t.Errorf("laws[0].Category = %q, want 'core'", laws[0].Category)
	}
	if laws[2].ID != "LAW-GL-001" {
		t.Errorf("laws[2].ID = %q, want 'LAW-GL-001'", laws[2].ID)
	}
	if laws[2].Category != "system" {
		t.Errorf("laws[2].Category = %q, want 'system'", laws[2].Category)
	}
}

func TestReadLaws_NoFile(t *testing.T) {
	tmpDir := t.TempDir()
	laws, err := ReadLaws(tmpDir)
	if err != nil {
		t.Fatalf("ReadLaws: %v", err)
	}
	if len(laws) != 0 {
		t.Fatalf("law count = %d, want 0", len(laws))
	}
}

func TestReadLaws_Fallback(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, ".aos"), 0755)
	os.WriteFile(filepath.Join(tmpDir, ".aos", "LAWS.md"), []byte(`# Laws

### Law 1: Immutable Rule
This rule cannot be changed.
`), 0644)

	laws, err := ReadLaws(tmpDir)
	if err != nil {
		t.Fatalf("ReadLaws: %v", err)
	}
	if len(laws) != 1 {
		t.Fatalf("law count = %d, want 1", len(laws))
	}
	if laws[0].Name != "Immutable Rule" {
		t.Errorf("laws[0].Name = %q, want 'Immutable Rule'", laws[0].Name)
	}
}

func TestCheckPhaseLock_AllPassed(t *testing.T) {
	gateMap := map[int]string{1: "npm run verify:1", 2: "npm run verify:2"}
	priorJobs := []JobPhaseInfo{
		{Name: "phase-1-store", Phase: 1, GateStatus: "passed", GateCommand: "npm run verify:1"},
	}
	check := CheckPhaseLock("phase-2-dispatch", 2, gateMap, priorJobs)
	if !check.Passed {
		t.Errorf("expected pass, got: %s", check.Reason)
	}
}

func TestCheckPhaseLock_PriorFailed(t *testing.T) {
	gateMap := map[int]string{1: "npm run verify:1", 2: "npm run verify:2"}
	priorJobs := []JobPhaseInfo{
		{Name: "phase-1-store", Phase: 1, GateStatus: "failed", GateCommand: "npm run verify:1"},
	}
	check := CheckPhaseLock("phase-2-dispatch", 2, gateMap, priorJobs)
	if check.Passed {
		t.Error("expected fail for prior phase with failed gate")
	}
	if check.LawID != "PHASE-LOCK" {
		t.Errorf("LawID = %q, want 'PHASE-LOCK'", check.LawID)
	}
}

func TestCheckPhaseLock_PriorNotRun(t *testing.T) {
	gateMap := map[int]string{1: "npm run verify:1", 2: "npm run verify:2"}
	priorJobs := []JobPhaseInfo{
		{Name: "phase-1-store", Phase: 1, GateStatus: "", GateCommand: "npm run verify:1"},
	}
	check := CheckPhaseLock("phase-2-dispatch", 2, gateMap, priorJobs)
	if check.Passed {
		t.Error("expected fail for prior phase with empty gate status")
	}
}

func TestCheckPhaseLock_Phase1(t *testing.T) {
	gateMap := map[int]string{1: "npm run verify:1"}
	check := CheckPhaseLock("phase-1-store", 1, gateMap, nil)
	if !check.Passed {
		t.Errorf("phase 1 should pass: %s", check.Reason)
	}
}

func TestCheckPhaseLock_UnknownPhase(t *testing.T) {
	gateMap := map[int]string{1: "npm run verify:1"}
	check := CheckPhaseLock("mystery-job", -1, gateMap, nil)
	if !check.Passed {
		t.Errorf("negative phase should be exempt: %s", check.Reason)
	}
}

func TestCheckGateIntegrity_GateSetAndDefined(t *testing.T) {
	gateMap := map[int]string{1: "npm run verify:1"}
	check := CheckGateIntegrity("phase-1-store", 1, gateMap, "npm run verify:1")
	if !check.Passed {
		t.Errorf("expected pass: %s", check.Reason)
	}
}

func TestCheckGateIntegrity_GateDefinedButEmpty(t *testing.T) {
	gateMap := map[int]string{1: "npm run verify:1"}
	check := CheckGateIntegrity("phase-1-store", 1, gateMap, "")
	if check.Passed {
		t.Error("expected fail for phase with gate defined but no gate command on job")
	}
	if check.LawID != "GATE-INTEGRITY" {
		t.Errorf("LawID = %q, want 'GATE-INTEGRITY'", check.LawID)
	}
}

func TestCheckGateIntegrity_NoGateForPhase(t *testing.T) {
	gateMap := map[int]string{1: "npm run verify:1"}
	check := CheckGateIntegrity("phase-2-dispatch", 2, gateMap, "")
	if !check.Passed {
		t.Errorf("expected pass for phase without gate requirement: %s", check.Reason)
	}
}

func TestCheckGateIntegrity_UnknownPhase(t *testing.T) {
	gateMap := map[int]string{1: "npm run verify:1"}
	check := CheckGateIntegrity("mystery-job", -1, gateMap, "")
	if !check.Passed {
		t.Errorf("negative phase should be exempt: %s", check.Reason)
	}
}
