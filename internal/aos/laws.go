package aos

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Law represents a parsed governance law from LAWSET.md or LAWS.md.
type Law struct {
	ID       string
	Name     string
	Rule     string
	Category string
}

// GovernanceCheck records the result of a governance check against a law.
type GovernanceCheck struct {
	LawID  string
	Passed bool
	Reason string
}

// JobPhaseInfo provides phase metadata for governance checks.
type JobPhaseInfo struct {
	Name        string
	Phase       int
	GateStatus  string
	GateCommand string
}

var lawHeaderRe = regexp.MustCompile(`^###\s+Law\s+(\d+):\s*(.+)`)
var systemLawRowRe = regexp.MustCompile(`^\|\s*(LAW-\S+)\s*\|\s*(.+?)\s*\|`)

// ReadLaws parses governance laws from the project directory.
// Primary: <projectDir>/governance/laws/LAWSET.md
// Fallback: <projectDir>/.aos/LAWS.md
// Returns empty slice if neither exists.
func ReadLaws(projectDir string) ([]Law, error) {
	primary := filepath.Join(projectDir, "governance", "laws", "LAWSET.md")
	fallback := filepath.Join(projectDir, ".aos", "LAWS.md")

	path := primary
	f, err := os.Open(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		path = fallback
		f, err = os.Open(path)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, nil
			}
			return nil, err
		}
	}
	defer f.Close()

	var laws []Law
	scanner := bufio.NewScanner(f)
	var pendingLaw *Law

	for scanner.Scan() {
		line := scanner.Text()

		// Flush pending law if we hit a new header or system-law row
		if pendingLaw != nil {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "|") {
				if pendingLaw.Rule == "" {
					pendingLaw.Rule = pendingLaw.Name
				}
				laws = append(laws, *pendingLaw)
				pendingLaw = nil
			} else {
				pendingLaw.Rule = trimmed
				laws = append(laws, *pendingLaw)
				pendingLaw = nil
				continue
			}
		}

		// Check for ### Law N: Name headers
		if m := lawHeaderRe.FindStringSubmatch(line); m != nil {
			pendingLaw = &Law{
				ID:       "LAW-" + m[1],
				Name:     strings.TrimSpace(m[2]),
				Category: "core",
			}
			continue
		}

		// Check for system-law table rows: | LAW-XX-NNN | Rule text |
		if m := systemLawRowRe.FindStringSubmatch(line); m != nil {
			id := strings.TrimSpace(m[1])
			rule := strings.TrimSpace(m[2])
			// Skip header/separator rows
			if strings.Contains(id, "---") || id == "ID" {
				continue
			}
			laws = append(laws, Law{
				ID:       id,
				Name:     id,
				Rule:     rule,
				Category: "system",
			})
		}
	}

	// Flush trailing pending law
	if pendingLaw != nil {
		if pendingLaw.Rule == "" {
			pendingLaw.Rule = pendingLaw.Name
		}
		laws = append(laws, *pendingLaw)
	}

	return laws, scanner.Err()
}

// CheckPhaseLock verifies that all prior phases have passed their gates
// before allowing a job in the given phase to execute.
func CheckPhaseLock(jobName string, phase int, gateMap map[int]string, priorJobs []JobPhaseInfo) GovernanceCheck {
	if phase < 0 {
		return GovernanceCheck{LawID: "PHASE-LOCK", Passed: true, Reason: "phase exempt"}
	}
	if phase <= 1 {
		return GovernanceCheck{LawID: "PHASE-LOCK", Passed: true, Reason: "phase 1 has no prior requirements"}
	}

	for p := 1; p < phase; p++ {
		if _, exists := gateMap[p]; !exists {
			continue
		}
		for _, j := range priorJobs {
			if j.Phase != p {
				continue
			}
			if j.GateCommand == "" {
				continue
			}
			if j.GateStatus != "passed" {
				return GovernanceCheck{
					LawID:  "PHASE-LOCK",
					Passed: false,
					Reason: fmt.Sprintf("job %q blocked: phase %d job %q gate not passed (status: %q)", jobName, p, j.Name, j.GateStatus),
				}
			}
		}
	}

	return GovernanceCheck{LawID: "PHASE-LOCK", Passed: true, Reason: "all prior phase gates passed"}
}

// CheckGateIntegrity verifies that a job in a gated phase has a gate command configured.
func CheckGateIntegrity(jobName string, phase int, gateMap map[int]string, gateCommand string) GovernanceCheck {
	if phase < 0 {
		return GovernanceCheck{LawID: "GATE-INTEGRITY", Passed: true, Reason: "phase exempt"}
	}

	if _, exists := gateMap[phase]; exists {
		if strings.TrimSpace(gateCommand) == "" {
			return GovernanceCheck{
				LawID:  "GATE-INTEGRITY",
				Passed: false,
				Reason: fmt.Sprintf("job %q in phase %d requires a gate command but none is configured", jobName, phase),
			}
		}
	}

	return GovernanceCheck{LawID: "GATE-INTEGRITY", Passed: true, Reason: "gate command present or not required"}
}
