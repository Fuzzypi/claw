package context

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	fileCreatedRe  = regexp.MustCompile(`^(?:Created|Modified|Removed):\s*(.+)`)
	gitDiffRe      = regexp.MustCompile(`^diff --git a/(\S+)`)
	gitPlusPlusRe  = regexp.MustCompile(`^\+\+\+ b/(\S+)`)
	gitMinusMinRe  = regexp.MustCompile(`^--- a/(\S+)`)
	fileModeRe     = regexp.MustCompile(`(?i)^\s*(create|modify|delete|rename)\s+mode\s`)
	testPassedRe   = regexp.MustCompile(`(?i)(\d+)\s+(?:tests?\s+)?pass(?:ed)?`)
	testFailedRe   = regexp.MustCompile(`(?i)(\d+)\s+(?:tests?\s+)?fail(?:ed)?`)
	goTestOkRe     = regexp.MustCompile(`(?m)^ok\s+\S+\s+[\d.]+s$`)
	goTestPassRe   = regexp.MustCompile(`(?m)^PASS$`)
	goTestFailRe   = regexp.MustCompile(`(?m)^FAIL$`)
	errorLineRe    = regexp.MustCompile(`(?i)(?:error|Error|ERROR|panic|FAIL)`)
)

// ExtractContext builds a structured context summary from job output.
func ExtractContext(jobName string, output string, exitCode int, gateStatus string) string {
	var sb strings.Builder

	// Status
	status := "PASSED"
	if exitCode != 0 {
		status = fmt.Sprintf("FAILED (exit code %d)", exitCode)
	}

	// Test summary
	testSummary := extractTestSummary(output)

	// Gate info
	gateInfo := ""
	if gateStatus != "" {
		gateInfo = fmt.Sprintf("gate %s", gateStatus)
	}

	// Build status line
	sb.WriteString(fmt.Sprintf("Job %q: %s", jobName, status))
	parts := []string{}
	if testSummary != "" {
		parts = append(parts, testSummary)
	}
	if gateInfo != "" {
		parts = append(parts, gateInfo)
	}
	if len(parts) > 0 {
		sb.WriteString(" — ")
		sb.WriteString(strings.Join(parts, ", "))
	}
	sb.WriteString("\n")

	// Files
	files := extractFiles(output)
	if len(files) > 0 {
		sb.WriteString(fmt.Sprintf("    Files: %s\n", strings.Join(files, ", ")))
	} else {
		sb.WriteString("    Files: none detected\n")
	}

	// Errors (only if failed)
	if exitCode != 0 {
		errors := extractErrors(output)
		if len(errors) > 0 {
			sb.WriteString("    Errors:\n")
			for _, e := range errors {
				sb.WriteString(fmt.Sprintf("      %s\n", e))
			}
		}
	}

	return sb.String()
}

func extractTestSummary(output string) string {
	parts := []string{}

	if m := testPassedRe.FindStringSubmatch(output); len(m) > 1 {
		parts = append(parts, fmt.Sprintf("%s tests passed", m[1]))
	}
	if m := testFailedRe.FindStringSubmatch(output); len(m) > 1 {
		parts = append(parts, fmt.Sprintf("%s tests failed", m[1]))
	}

	// Go test package results
	if len(parts) == 0 {
		okCount := len(goTestOkRe.FindAllString(output, -1))
		passCount := len(goTestPassRe.FindAllString(output, -1))
		failCount := len(goTestFailRe.FindAllString(output, -1))

		if okCount > 0 {
			parts = append(parts, fmt.Sprintf("%d packages ok", okCount))
		}
		if passCount > 0 && okCount == 0 {
			parts = append(parts, "PASS")
		}
		if failCount > 0 {
			parts = append(parts, "FAIL")
		}
	}

	return strings.Join(parts, ", ")
}

func extractFiles(output string) []string {
	seen := map[string]bool{}
	var files []string

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		var path string

		if m := fileCreatedRe.FindStringSubmatch(line); len(m) > 1 {
			path = strings.TrimSpace(m[1])
		} else if m := gitDiffRe.FindStringSubmatch(line); len(m) > 1 {
			path = m[1]
		} else if m := gitPlusPlusRe.FindStringSubmatch(line); len(m) > 1 {
			path = m[1]
		} else if m := gitMinusMinRe.FindStringSubmatch(line); len(m) > 1 {
			path = m[1]
		} else if fileModeRe.MatchString(line) {
			// File mode line — no extractable path typically
			continue
		}

		if path != "" && !seen[path] {
			seen[path] = true
			files = append(files, path)
			if len(files) >= 20 {
				break
			}
		}
	}

	return files
}

func extractErrors(output string) []string {
	seen := map[string]bool{}
	var errors []string

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if errorLineRe.MatchString(trimmed) && !seen[trimmed] {
			seen[trimmed] = true
			errors = append(errors, trimmed)
			if len(errors) >= 5 {
				break
			}
		}
	}

	return errors
}
