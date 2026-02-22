package aos

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadGates(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, ".aos"), 0755)
	os.WriteFile(filepath.Join(tmpDir, ".aos", "GATES.md"), []byte(`# Gates

| Gate | Script | Phase |
|------|--------|-------|
| store | `+"`verify/claw_phase01_store.verify.cjs`"+` | 1 |
| dispatch | `+"`verify/claw_phase02_dispatch.verify.cjs`"+` | 2 |
`), 0644)

	gates, err := ReadGates(tmpDir)
	if err != nil {
		t.Fatalf("ReadGates: %v", err)
	}
	if len(gates) != 2 {
		t.Fatalf("gate count = %d, want 2", len(gates))
	}
	if gates[0].Name != "store" {
		t.Errorf("gate[0].Name = %q, want 'store'", gates[0].Name)
	}
	if gates[0].Script != "verify/claw_phase01_store.verify.cjs" {
		t.Errorf("gate[0].Script = %q, want 'verify/claw_phase01_store.verify.cjs'", gates[0].Script)
	}
	if gates[0].Phase != 1 {
		t.Errorf("gate[0].Phase = %d, want 1", gates[0].Phase)
	}
	if gates[1].Name != "dispatch" {
		t.Errorf("gate[1].Name = %q, want 'dispatch'", gates[1].Name)
	}
	if gates[1].Phase != 2 {
		t.Errorf("gate[1].Phase = %d, want 2", gates[1].Phase)
	}
}

func TestReadGatesNoFile(t *testing.T) {
	tmpDir := t.TempDir()
	gates, err := ReadGates(tmpDir)
	if err != nil {
		t.Fatalf("ReadGates: %v", err)
	}
	if len(gates) != 0 {
		t.Fatalf("gate count = %d, want 0", len(gates))
	}
}

func TestReadGatesSkipsMalformedRows(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, ".aos"), 0755)
	os.WriteFile(filepath.Join(tmpDir, ".aos", "GATES.md"), []byte(`# Gates

| Gate | Script | Phase |
|------|--------|-------|
| store | `+"`verify/claw_phase01_store.verify.cjs`"+` | 1 |
| bad | `+"`verify/bad.cjs`"+` | notanumber |
| incomplete |
`), 0644)

	gates, err := ReadGates(tmpDir)
	if err != nil {
		t.Fatalf("ReadGates: %v", err)
	}
	if len(gates) != 1 {
		t.Fatalf("gate count = %d, want 1", len(gates))
	}
	if gates[0].Name != "store" {
		t.Errorf("gate[0].Name = %q, want 'store'", gates[0].Name)
	}
}

func TestReadPhases(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "docs", "plan"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "docs", "plan", "CLAW_PHASE_PLAN.md"), []byte(`# Phase Plan

## Phase 0 — Foundation _(complete)_

Some details.

## Phase 1 — Store + Job Model

More details.

## Phase 2 — Dispatch Engine

Even more.
`), 0644)

	phases, err := ReadPhases(tmpDir)
	if err != nil {
		t.Fatalf("ReadPhases: %v", err)
	}
	if len(phases) != 2 {
		t.Fatalf("phase count = %d, want 2", len(phases))
	}
	if phases[0].Number != 1 {
		t.Errorf("phase[0].Number = %d, want 1", phases[0].Number)
	}
	if phases[0].Name != "Store + Job Model" {
		t.Errorf("phase[0].Name = %q, want 'Store + Job Model'", phases[0].Name)
	}
	if phases[1].Number != 2 {
		t.Errorf("phase[1].Number = %d, want 2", phases[1].Number)
	}
	if phases[1].Name != "Dispatch Engine" {
		t.Errorf("phase[1].Name = %q, want 'Dispatch Engine'", phases[1].Name)
	}
}

func TestReadPhasesNoFile(t *testing.T) {
	tmpDir := t.TempDir()
	phases, err := ReadPhases(tmpDir)
	if err != nil {
		t.Fatalf("ReadPhases: %v", err)
	}
	if len(phases) != 0 {
		t.Fatalf("phase count = %d, want 0", len(phases))
	}
}

func TestReadVerifyScripts(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(`{
  "scripts": {
    "verify:claw:phase01": "node verify/claw_phase01_store.verify.cjs",
    "verify:claw:phase02": "node verify/claw_phase02_dispatch.verify.cjs",
    "build": "echo build"
  }
}`), 0644)

	scripts, err := ReadVerifyScripts(tmpDir)
	if err != nil {
		t.Fatalf("ReadVerifyScripts: %v", err)
	}
	if len(scripts) != 2 {
		t.Fatalf("script count = %d, want 2", len(scripts))
	}
	// Sorted by key
	if scripts[0].Key != "verify:claw:phase01" {
		t.Errorf("scripts[0].Key = %q, want 'verify:claw:phase01'", scripts[0].Key)
	}
	if scripts[0].Command != "node verify/claw_phase01_store.verify.cjs" {
		t.Errorf("scripts[0].Command = %q", scripts[0].Command)
	}
	if scripts[1].Key != "verify:claw:phase02" {
		t.Errorf("scripts[1].Key = %q, want 'verify:claw:phase02'", scripts[1].Key)
	}
}

func TestReadVerifyScriptsNoFile(t *testing.T) {
	tmpDir := t.TempDir()
	scripts, err := ReadVerifyScripts(tmpDir)
	if err != nil {
		t.Fatalf("ReadVerifyScripts: %v", err)
	}
	if len(scripts) != 0 {
		t.Fatalf("script count = %d, want 0", len(scripts))
	}
}

func TestMapGatesToScripts(t *testing.T) {
	gates := []GateEntry{
		{Name: "store", Script: "verify/claw_phase01_store.verify.cjs", Phase: 1},
	}
	scripts := []VerifyScript{
		{Key: "verify:claw:phase01", Command: "node verify/claw_phase01_store.verify.cjs"},
	}

	m := MapGatesToScripts(gates, scripts)
	if len(m) != 1 {
		t.Fatalf("map size = %d, want 1", len(m))
	}
	if m[1] != "npm run verify:claw:phase01" {
		t.Errorf("m[1] = %q, want 'npm run verify:claw:phase01'", m[1])
	}
}

func TestMapGatesToScriptsNoMatch(t *testing.T) {
	gates := []GateEntry{
		{Name: "store", Script: "verify/nonexistent.cjs", Phase: 1},
	}
	scripts := []VerifyScript{
		{Key: "verify:claw:phase01", Command: "node verify/claw_phase01_store.verify.cjs"},
	}

	m := MapGatesToScripts(gates, scripts)
	if len(m) != 0 {
		t.Fatalf("map size = %d, want 0", len(m))
	}
}

func TestReadProject(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, ".aos"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "docs", "plan"), 0755)

	os.WriteFile(filepath.Join(tmpDir, ".aos", "GATES.md"), []byte(`| Gate | Script | Phase |
|------|--------|-------|
| store | `+"`verify/claw_phase01_store.verify.cjs`"+` | 1 |
`), 0644)

	os.WriteFile(filepath.Join(tmpDir, "docs", "plan", "TEST_PHASE_PLAN.md"), []byte(`## Phase 1 — Store + Job Model
`), 0644)

	os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(`{
  "scripts": {
    "verify:claw:phase01": "node verify/claw_phase01_store.verify.cjs"
  }
}`), 0644)

	info, err := ReadProject(tmpDir)
	if err != nil {
		t.Fatalf("ReadProject: %v", err)
	}
	if len(info.Gates) != 1 {
		t.Errorf("gates = %d, want 1", len(info.Gates))
	}
	if len(info.Phases) != 1 {
		t.Errorf("phases = %d, want 1", len(info.Phases))
	}
	if len(info.Scripts) != 1 {
		t.Errorf("scripts = %d, want 1", len(info.Scripts))
	}
}
