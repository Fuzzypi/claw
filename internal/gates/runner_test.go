package gates

import (
	"strings"
	"testing"
	"time"
)

func TestRunGatePass(t *testing.T) {
	result, err := RunGate("echo all good && exit 0", "/tmp", 10*time.Second)
	if err != nil {
		t.Fatalf("RunGate: %v", err)
	}
	if !result.Passed {
		t.Error("expected Passed=true")
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
	if !strings.Contains(result.Output, "all good") {
		t.Errorf("Output = %q, want containing 'all good'", result.Output)
	}
}

func TestRunGateFail(t *testing.T) {
	result, err := RunGate("echo FAIL && exit 1", "/tmp", 10*time.Second)
	if err != nil {
		t.Fatalf("RunGate: %v", err)
	}
	if result.Passed {
		t.Error("expected Passed=false")
	}
	if result.ExitCode != 1 {
		t.Errorf("ExitCode = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(result.Output, "FAIL") {
		t.Errorf("Output = %q, want containing 'FAIL'", result.Output)
	}
}

func TestRunGateExitCodeAuthoritative(t *testing.T) {
	// Checkmarks in output must NOT override exit code
	result, err := RunGate("printf '\\xe2\\x9c\\x93 all checks passed' && exit 1", "/tmp", 10*time.Second)
	if err != nil {
		t.Fatalf("RunGate: %v", err)
	}
	if result.Passed {
		t.Error("expected Passed=false — exit code must override checkmarks")
	}
	if result.ExitCode != 1 {
		t.Errorf("ExitCode = %d, want 1", result.ExitCode)
	}
}

func TestRunGateTimeout(t *testing.T) {
	start := time.Now()
	result, err := RunGate("sleep 10", "/tmp", 1*time.Second)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("RunGate: %v", err)
	}
	if result.Passed {
		t.Error("expected Passed=false for timeout")
	}
	if elapsed > 5*time.Second {
		t.Errorf("took %v, expected < 5s", elapsed)
	}
}

func TestRunGateOutputTruncation(t *testing.T) {
	result, err := RunGate("dd if=/dev/zero bs=1 count=2000000 2>/dev/null | tr '\\0' 'x'", "/tmp", 30*time.Second)
	if err != nil {
		t.Fatalf("RunGate: %v", err)
	}
	if len(result.Output) > 1_100_000 {
		t.Errorf("output len = %d, want < 1.1MB", len(result.Output))
	}
	if !strings.Contains(result.Output, "[... truncated") {
		t.Error("truncation marker not found")
	}
}
