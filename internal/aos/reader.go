package aos

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// GateEntry represents a row from the GATES.md gate table.
type GateEntry struct {
	Name   string
	Script string
	Phase  int
}

// PhaseEntry represents a phase header from the phase plan.
type PhaseEntry struct {
	Number int
	Name   string
}

// VerifyScript represents a verify:* script from package.json.
type VerifyScript struct {
	Key     string
	Command string
}

// ProjectInfo holds all parsed AOS governance data for a project.
type ProjectInfo struct {
	Gates   []GateEntry
	Phases  []PhaseEntry
	Scripts []VerifyScript
}

// ReadProject orchestrates reading all AOS governance files from a project directory.
// Missing files are silently skipped.
func ReadProject(projectDir string) (*ProjectInfo, error) {
	gates, err := ReadGates(projectDir)
	if err != nil {
		return nil, fmt.Errorf("reading gates: %w", err)
	}

	phases, err := ReadPhases(projectDir)
	if err != nil {
		return nil, fmt.Errorf("reading phases: %w", err)
	}

	scripts, err := ReadVerifyScripts(projectDir)
	if err != nil {
		return nil, fmt.Errorf("reading verify scripts: %w", err)
	}

	return &ProjectInfo{
		Gates:   gates,
		Phases:  phases,
		Scripts: scripts,
	}, nil
}

// ReadGates parses <projectDir>/.aos/GATES.md for gate table entries.
func ReadGates(projectDir string) ([]GateEntry, error) {
	path := filepath.Join(projectDir, ".aos", "GATES.md")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var gates []GateEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, "|") {
			continue
		}
		// Skip header/separator rows
		if strings.Contains(line, "---") {
			continue
		}
		if strings.Contains(line, "Gate") && strings.Contains(line, "Script") {
			continue
		}
		if strings.Contains(line, "Gate") && strings.Contains(line, "Phase") {
			continue
		}

		parts := strings.Split(line, "|")
		// Need at least 4 parts for name, script, phase (with leading/trailing empty from |)
		if len(parts) < 4 {
			continue
		}

		// Find the columns: strip leading/trailing empty segments
		var cols []string
		for _, p := range parts {
			trimmed := strings.TrimSpace(p)
			if trimmed != "" {
				cols = append(cols, trimmed)
			}
		}

		if len(cols) < 3 {
			continue
		}

		// The gate table may have 3 or 4 columns (with or without "Checks" column).
		// We need: name (col 0), script (col 1), phase (col 2)
		name := cols[0]
		script := strings.Trim(cols[1], "`")

		// Find the phase column — it might be col 2 or later
		phaseStr := ""
		for i := 2; i < len(cols); i++ {
			if _, err := strconv.Atoi(strings.TrimSpace(cols[i])); err == nil {
				phaseStr = strings.TrimSpace(cols[i])
				break
			}
		}
		if phaseStr == "" {
			continue
		}

		phase, err := strconv.Atoi(phaseStr)
		if err != nil {
			continue
		}

		gates = append(gates, GateEntry{
			Name:   name,
			Script: script,
			Phase:  phase,
		})
	}

	sort.Slice(gates, func(i, j int) bool {
		return gates[i].Phase < gates[j].Phase
	})

	return gates, scanner.Err()
}

var phaseHeaderRe = regexp.MustCompile(`^##\s+Phase\s+(\d+)\s*[—–\-]\s*(.+)`)
var completeRe = regexp.MustCompile(`(?i)_\(complete\)_`)

// ReadPhases parses phase headers from the project's phase plan file.
func ReadPhases(projectDir string) ([]PhaseEntry, error) {
	// Look for files matching docs/plan/*PHASE_PLAN*.md
	pattern := filepath.Join(projectDir, "docs", "plan", "*PHASE_PLAN*.md")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, nil
	}

	path := matches[0]
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var phases []PhaseEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		m := phaseHeaderRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}

		// Skip completed phases
		if completeRe.MatchString(line) {
			continue
		}

		num, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}

		name := strings.TrimSpace(m[2])
		phases = append(phases, PhaseEntry{
			Number: num,
			Name:   name,
		})
	}

	sort.Slice(phases, func(i, j int) bool {
		return phases[i].Number < phases[j].Number
	})

	return phases, scanner.Err()
}

// ReadVerifyScripts parses package.json for verify:* script entries.
func ReadVerifyScripts(projectDir string) ([]VerifyScript, error) {
	path := filepath.Join(projectDir, "package.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, fmt.Errorf("parsing package.json: %w", err)
	}

	var scripts []VerifyScript
	for key, cmd := range pkg.Scripts {
		if strings.HasPrefix(key, "verify:") {
			scripts = append(scripts, VerifyScript{
				Key:     key,
				Command: cmd,
			})
		}
	}

	sort.Slice(scripts, func(i, j int) bool {
		return scripts[i].Key < scripts[j].Key
	})

	return scripts, nil
}

// MapGatesToScripts maps gate entries to their npm script keys.
// Returns a map of phase number to the full npm run command.
func MapGatesToScripts(gates []GateEntry, scripts []VerifyScript) map[int]string {
	result := make(map[int]string)

	for _, gate := range gates {
		for _, script := range scripts {
			if strings.Contains(script.Command, gate.Script) {
				result[gate.Phase] = "npm run " + script.Key
				break
			}
		}
	}

	return result
}
