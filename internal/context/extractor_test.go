package context

import (
	"strings"
	"testing"
)

func TestExtractContextPassed(t *testing.T) {
	output := `=== RUN   TestCreateAndGetPipeline
--- PASS: TestCreateAndGetPipeline (0.00s)
ok  github.com/fuzzypi/claw/internal/store 0.015s
PASS
Created: internal/store/db.go
Modified: internal/store/jobs.go
18 tests passed`

	result := ExtractContext("phase-1", output, 0, "passed")

	if !strings.Contains(result, "PASSED") {
		t.Errorf("result missing 'PASSED': %s", result)
	}
	if !strings.Contains(result, "18 tests passed") {
		t.Errorf("result missing '18 tests passed': %s", result)
	}
	if !strings.Contains(result, "gate passed") {
		t.Errorf("result missing 'gate passed': %s", result)
	}
}

func TestExtractContextFailed(t *testing.T) {
	output := `dispatcher_test.go:45: expected 3, got 0
Error: connection refused
dispatcher_test.go:52: timeout exceeded
FAIL`

	result := ExtractContext("phase-2", output, 1, "")

	if !strings.Contains(result, "FAILED") {
		t.Errorf("result missing 'FAILED': %s", result)
	}
	if !strings.Contains(result, "Errors:") {
		t.Errorf("result missing 'Errors:': %s", result)
	}
	if !strings.Contains(result, "Error: connection refused") {
		t.Errorf("result missing error line: %s", result)
	}
}

func TestExtractContextFilesDetected(t *testing.T) {
	output := "Created: internal/store/db.go\nModified: internal/store/jobs.go\n"

	result := ExtractContext("phase-1", output, 0, "")

	if !strings.Contains(result, "internal/store/db.go") {
		t.Errorf("result missing db.go: %s", result)
	}
	if !strings.Contains(result, "internal/store/jobs.go") {
		t.Errorf("result missing jobs.go: %s", result)
	}
}

func TestExtractContextGoTestOutput(t *testing.T) {
	output := `=== RUN   TestFoo
--- PASS: TestFoo (0.01s)
ok  github.com/fuzzypi/claw/internal/store 0.015s
PASS`

	result := ExtractContext("phase-1", output, 0, "")

	if !strings.Contains(result, "PASSED") {
		t.Errorf("result missing 'PASSED': %s", result)
	}
	// Should detect Go test package results
	if !strings.Contains(result, "packages ok") && !strings.Contains(result, "PASS") {
		t.Errorf("result missing Go test detection: %s", result)
	}
}

func TestExtractContextNoTestResults(t *testing.T) {
	output := "Just some random output with no test patterns\nBuilding stuff...\nDone."

	result := ExtractContext("build-step", output, 0, "")

	if !strings.Contains(result, "PASSED") {
		t.Errorf("result missing 'PASSED': %s", result)
	}
	// Should still produce valid output
	if !strings.Contains(result, `Job "build-step"`) {
		t.Errorf("result missing job name: %s", result)
	}
}

func TestExtractContextMaxErrorLines(t *testing.T) {
	lines := []string{}
	for i := 0; i < 10; i++ {
		lines = append(lines, "Error: something went wrong "+string(rune('A'+i)))
	}
	output := strings.Join(lines, "\n")

	result := ExtractContext("failing", output, 1, "")

	// Should only include first 5
	errorCount := strings.Count(result, "Error: something went wrong")
	if errorCount > 5 {
		t.Errorf("error count = %d, want <= 5", errorCount)
	}
}
