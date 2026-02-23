package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/fuzzypi/claw/internal/aos"
	"github.com/fuzzypi/claw/internal/dispatch"
	"github.com/fuzzypi/claw/internal/engram"
	"github.com/fuzzypi/claw/internal/store"
	"github.com/spf13/cobra"
)

// RunCmd returns the `claw run <pipeline-id>` command.
// An optional engram.Client enables memory persistence across runs.
func RunCmd(s *store.Store, ec ...*engram.Client) *cobra.Command {
	return &cobra.Command{
		Use:   "run <pipeline-id>",
		Short: "Run a pipeline to completion",
		Long: "Execute all jobs in a pipeline in dependency order.\n" +
			"Jobs are dispatched to idle agents. Shell agents run automatically;\n" +
			"manual agents prompt for paste-back.\n\n" +
			"Example:\n  claw run 1",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid pipeline ID: %s", args[0])
			}

			p, err := s.GetPipeline(id)
			if err != nil {
				return fmt.Errorf("pipeline %d not found", id)
			}
			if p.Status != "active" {
				return fmt.Errorf("pipeline %d has status %q, expected \"active\"", id, p.Status)
			}

			idle, err := s.ListAgentsByStatus("idle")
			if err != nil {
				return err
			}
			if len(idle) == 0 {
				return fmt.Errorf("no idle agents registered — register at least one agent first")
			}

			var engramClient *engram.Client
			if len(ec) > 0 {
				engramClient = ec[0]
			}
			d := dispatch.NewDispatcher(s, engramClient)
			if err := d.Run(id); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Pipeline failed: %v\n", err)
				return err
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Pipeline completed successfully")
			return nil
		},
	}
}

// StatusCmd returns the `claw status <pipeline-id>` command.
func StatusCmd(s *store.Store) *cobra.Command {
	return &cobra.Command{
		Use:   "status <pipeline-id>",
		Short: "Show pipeline job status",
		Long: "Show the status of all jobs in a pipeline.\n\n" +
			"Example:\n  claw status 1",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid pipeline ID: %s", args[0])
			}

			p, err := s.GetPipeline(id)
			if err != nil {
				return fmt.Errorf("pipeline %d not found", id)
			}

			jobs, err := s.ListJobsByPipeline(id)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Pipeline: %s (status: %s)\n\n", p.Name, p.Status)

			w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tName\tStatus\tAgent\tExit Code\tAttempts\tStarted\tCompleted")
			fmt.Fprintln(w, "--\t----\t------\t-----\t---------\t--------\t-------\t---------")

			completed, failed, pending := 0, 0, 0
			for _, j := range jobs {
				agentStr := "-"
				if j.AgentID != nil {
					agentStr = fmt.Sprintf("%d", *j.AgentID)
				}
				exitStr := "-"
				if j.ExitCode != nil {
					exitStr = fmt.Sprintf("%d", *j.ExitCode)
				}
				startedStr := "-"
				if j.StartedAt != nil {
					startedStr = j.StartedAt.Format("15:04:05")
				}
				completedStr := "-"
				if j.CompletedAt != nil {
					completedStr = j.CompletedAt.Format("15:04:05")
				}

				fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%d\t%s\t%s\n",
					j.ID, j.Name, j.Status, agentStr, exitStr, j.AttemptCount, startedStr, completedStr)

				switch j.Status {
				case "completed":
					completed++
				case "failed":
					failed++
				default:
					pending++
				}
			}
			w.Flush()

			fmt.Fprintf(out, "\n%d completed, %d failed, %d pending\n", completed, failed, pending)
			return nil
		},
	}
}

// PipelineCreateCmd returns the `claw pipeline create` command.
func PipelineCreateCmd(s *store.Store) *cobra.Command {
	var projectDir string

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new pipeline",
		Long: "Create an empty pipeline for a project directory.\n\n" +
			"Example:\n  claw pipeline create my-project --project ~/my-project",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			p, err := s.CreatePipeline(name, projectDir)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created pipeline %d: %s\n", p.ID, p.Name)
			return nil
		},
	}
	cmd.Flags().StringVar(&projectDir, "project", "", "Project directory (required)")
	cmd.MarkFlagRequired("project")
	return cmd
}

// JobAddCmd returns the `claw job add` command.
func JobAddCmd(s *store.Store) *cobra.Command {
	var promptFile string
	var dependsOn []string
	var gate string
	var phase int

	cmd := &cobra.Command{
		Use:   "add <pipeline-id> <name>",
		Short: "Add a job to a pipeline",
		Long: "Add a job with a prompt file to a pipeline.\n" +
			"Optionally specify dependencies, a gate command, and phase number.\n\n" +
			"Example:\n  claw job add 1 phase-1 --prompt-file prompt.txt --gate \"npm run verify:phase01\" --phase 1",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			pipelineID, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid pipeline ID: %s", args[0])
			}
			name := args[1]

			promptData, err := os.ReadFile(promptFile)
			if err != nil {
				return fmt.Errorf("reading prompt file: %w", err)
			}

			var phasePtr *int
			if phase >= 0 {
				phasePtr = &phase
			}

			j, err := s.CreateJobWithPhase(pipelineID, name, string(promptData), phasePtr)
			if err != nil {
				return err
			}

			if gate != "" {
				if err := s.SetJobGateCommand(j.ID, gate); err != nil {
					return err
				}
			}

			for _, depName := range dependsOn {
				dep, err := s.GetJobByName(pipelineID, depName)
				if err != nil {
					return fmt.Errorf("dependency %q: %w", depName, err)
				}
				if err := s.AddDependency(j.ID, dep.ID); err != nil {
					return fmt.Errorf("adding dependency on %q: %w", depName, err)
				}
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Added job %d: %s to pipeline %d\n", j.ID, j.Name, pipelineID)
			return nil
		},
	}
	cmd.Flags().StringVar(&promptFile, "prompt-file", "", "Path to prompt file (required)")
	cmd.MarkFlagRequired("prompt-file")
	cmd.Flags().StringArrayVar(&dependsOn, "depends-on", nil, "Job name this depends on (repeatable)")
	cmd.Flags().StringVar(&gate, "gate", "", "Gate command to run after job completes")
	cmd.Flags().IntVar(&phase, "phase", -1, "Phase number for governance enforcement")
	return cmd
}

// AgentRegisterCmd returns the `claw agent register` command.
func AgentRegisterCmd(s *store.Store) *cobra.Command {
	var agentType string
	var command string
	var agentArgs []string
	var cwd string
	var timeout int

	cmd := &cobra.Command{
		Use:   "register <name>",
		Short: "Register an agent",
		Long: "Register a shell or manual agent for job execution.\n" +
			"Shell agents require --command; manual agents prompt for paste-back.\n\n" +
			"Examples:\n" +
			"  claw agent register builder-1 --type shell --command claude\n" +
			"  claw agent register human-1 --type manual",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			if agentType == "shell" && command == "" {
				return fmt.Errorf("shell agents require --command")
			}

			var cmdPtr *string
			if command != "" {
				cmdPtr = &command
			}
			var cwdPtr *string
			if cwd != "" {
				cwdPtr = &cwd
			}
			var timeoutPtr *int
			if cmd.Flags().Changed("timeout") {
				timeoutPtr = &timeout
			}

			a, err := s.RegisterAgent(name, agentType, cmdPtr, agentArgs, cwdPtr, timeoutPtr)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Registered agent %d: %s (type: %s)\n", a.ID, a.Name, a.Type)
			return nil
		},
	}
	cmd.Flags().StringVar(&agentType, "type", "", "Agent type: shell or manual (required)")
	cmd.MarkFlagRequired("type")
	cmd.Flags().StringVar(&command, "command", "", "Command to run (required for shell)")
	cmd.Flags().StringArrayVar(&agentArgs, "args", nil, "Command arguments (repeatable)")
	cmd.Flags().StringVar(&cwd, "cwd", "", "Working directory")
	cmd.Flags().IntVar(&timeout, "timeout", 600, "Timeout in seconds")
	return cmd
}

// AgentListCmd returns the `claw agent list` command.
func AgentListCmd(s *store.Store) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all agents",
		Long: "Show all registered agents with their status and current job.\n\n" +
			"Example:\n  claw agent list",
		RunE: func(cmd *cobra.Command, args []string) error {
			agents, err := s.ListAgents()
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tName\tType\tStatus\tCommand\tCurrent Job")
			fmt.Fprintln(w, "--\t----\t----\t------\t-------\t-----------")

			for _, a := range agents {
				cmdStr := "-"
				if a.Command != nil {
					cmdStr = *a.Command
				}
				jobStr := "-"
				if a.CurrentJobID != nil {
					jobStr = fmt.Sprintf("%d", *a.CurrentJobID)
				}
				fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\n",
					a.ID, a.Name, a.Type, a.Status, cmdStr, jobStr)
			}
			w.Flush()
			return nil
		},
	}
}

// OutputCmd returns the `claw output <job-id>` command.
func OutputCmd(s *store.Store) *cobra.Command {
	return &cobra.Command{
		Use:   "output <job-id>",
		Short: "Show captured output for a job",
		Long: "Display the captured output from a completed job.\n\n" +
			"Example:\n  claw output 3",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid job ID: %s", args[0])
			}

			j, err := s.GetJob(id)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			if j.Output == nil {
				fmt.Fprintln(out, "No output captured")
			} else {
				fmt.Fprintln(out, *j.Output)
			}
			return nil
		},
	}
}

// SummaryCmd returns the `claw summary <pipeline-id>` command.
func SummaryCmd(s *store.Store) *cobra.Command {
	return &cobra.Command{
		Use:   "summary <pipeline-id>",
		Short: "Generate handoff summary from pipeline context",
		Long: "Generate a handoff summary from accumulated pipeline context.\n\n" +
			"Example:\n  claw summary 1",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid pipeline ID: %s", args[0])
			}

			p, err := s.GetPipeline(id)
			if err != nil {
				return fmt.Errorf("pipeline %d not found", id)
			}

			entries, err := s.ListContextByPipeline(id)
			if err != nil {
				return err
			}

			jobs, err := s.ListJobsByPipeline(id)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "=== Handoff Summary: %s ===\n\n", p.Name)

			for i, e := range entries {
				fmt.Fprintf(out, "[%d] %s\n", i+1, e.Content)
			}

			completed, failed := 0, 0
			for _, j := range jobs {
				switch j.Status {
				case "completed":
					completed++
				case "failed":
					failed++
				}
			}

			fmt.Fprintf(out, "\nPipeline status: %s\n", p.Status)
			fmt.Fprintf(out, "Total jobs: %d | Completed: %d | Failed: %d\n", len(jobs), completed, failed)
			return nil
		},
	}
}

// ContextCmd returns the `claw context <pipeline-id>` command.
func ContextCmd(s *store.Store) *cobra.Command {
	return &cobra.Command{
		Use:   "context <pipeline-id>",
		Short: "Show accumulated context entries",
		Long: "Show all accumulated context entries for a pipeline.\n\n" +
			"Example:\n  claw context 1",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid pipeline ID: %s", args[0])
			}

			entries, err := s.ListContextByPipeline(id)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			if len(entries) == 0 {
				fmt.Fprintln(out, "No context entries")
				return nil
			}

			for i, e := range entries {
				fmt.Fprintf(out, "[%d] %s — %s\n", i+1, e.CreatedAt.Format("15:04:05"), e.Content)
			}
			return nil
		},
	}
}

// GateCmd returns the `claw gate <job-id>` command.
func GateCmd(s *store.Store) *cobra.Command {
	return &cobra.Command{
		Use:   "gate <job-id>",
		Short: "Show gate result for a job",
		Long: "Show the gate result (status, exit code, output) for a job.\n\n" +
			"Example:\n  claw gate 3",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid job ID: %s", args[0])
			}

			j, err := s.GetJob(id)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			if j.GateStatus == nil {
				fmt.Fprintln(out, "No gate configured for this job")
				return nil
			}

			fmt.Fprintf(out, "Gate status: %s\n", *j.GateStatus)
			if j.GateExitCode != nil {
				fmt.Fprintf(out, "Exit code: %d\n", *j.GateExitCode)
			}
			if j.GateOutput != nil {
				fmt.Fprintf(out, "Output:\n%s\n", *j.GateOutput)
			}
			return nil
		},
	}
}

// LogCmd returns the `claw log` command.
func LogCmd(s *store.Store) *cobra.Command {
	var pipelineID int64
	var limit int

	cmd := &cobra.Command{
		Use:   "log",
		Short: "Show activity log",
		Long: "Show chronological activity log for pipeline execution.\n\n" +
			"Examples:\n  claw log\n  claw log --pipeline 1 --limit 20",
		RunE: func(cmd *cobra.Command, args []string) error {
			var pidPtr *int64
			if cmd.Flags().Changed("pipeline") {
				pidPtr = &pipelineID
			}

			entries, err := s.ListActivity(pidPtr, limit)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			if len(entries) == 0 {
				fmt.Fprintln(out, "No activity recorded")
				return nil
			}

			for _, e := range entries {
				fmt.Fprintf(out, "%s  %s\n", e.CreatedAt.Format("2006-01-02 15:04:05"), e.Detail)
			}
			return nil
		},
	}
	cmd.Flags().Int64Var(&pipelineID, "pipeline", 0, "Filter by pipeline ID")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum entries to show")
	return cmd
}

var sanitizeRe = regexp.MustCompile(`[^a-z0-9-]+`)

func sanitizeName(name string) string {
	s := strings.ToLower(name)
	s = strings.ReplaceAll(s, " ", "-")
	s = sanitizeRe.ReplaceAllString(s, "")
	s = strings.Trim(s, "-")
	return s
}

// InitCmd returns the `claw init <project-dir>` command.
func InitCmd(s *store.Store) *cobra.Command {
	return &cobra.Command{
		Use:   "init <project-dir>",
		Short: "Generate a pipeline from AOS governance files",
		Long: "Scan an AOS project directory and generate a pipeline from\n" +
			"governance files (GATES.md, phase plan, package.json).\n\n" +
			"Example:\n  claw init ~/aos-platform",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectDir, err := filepath.Abs(args[0])
			if err != nil {
				return fmt.Errorf("resolving path: %w", err)
			}

			info, err := aos.ReadProject(projectDir)
			if err != nil {
				return err
			}

			if len(info.Phases) == 0 {
				return fmt.Errorf("no phases found in %s", projectDir)
			}

			gateMap := aos.MapGatesToScripts(info.Gates, info.Scripts)

			pipelineName := filepath.Base(projectDir)
			p, err := s.CreatePipeline(pipelineName, projectDir)
			if err != nil {
				return fmt.Errorf("creating pipeline: %w", err)
			}

			out := cmd.OutOrStdout()
			var prevJobID int64
			jobCount := 0
			gateCount := 0
			depCount := 0

			type jobInfo struct {
				number  int
				name    string
				hasGate bool
			}
			var jobInfos []jobInfo

			for _, phase := range info.Phases {
				jobName := fmt.Sprintf("phase-%d-%s", phase.Number, sanitizeName(phase.Name))
				prompt := fmt.Sprintf("Phase %d: %s", phase.Number, phase.Name)

				phaseNum := phase.Number
				j, err := s.CreateJobWithPhase(p.ID, jobName, prompt, &phaseNum)
				if err != nil {
					return fmt.Errorf("creating job %q: %w", jobName, err)
				}
				jobCount++

				hasGate := false
				if gateCmd, ok := gateMap[phase.Number]; ok {
					if err := s.SetJobGateCommand(j.ID, gateCmd); err != nil {
						return fmt.Errorf("setting gate for %q: %w", jobName, err)
					}
					gateCount++
					hasGate = true
				}

				if prevJobID > 0 {
					if err := s.AddDependency(j.ID, prevJobID); err != nil {
						return fmt.Errorf("adding dependency for %q: %w", jobName, err)
					}
					depCount++
				}

				jobInfos = append(jobInfos, jobInfo{
					number:  phase.Number,
					name:    jobName,
					hasGate: hasGate,
				})

				prevJobID = j.ID
			}

			fmt.Fprintf(out, "Initialized pipeline %d: %s\n", p.ID, pipelineName)
			fmt.Fprintf(out, "  Jobs: %d\n", jobCount)
			fmt.Fprintf(out, "  With gates: %d\n", gateCount)
			fmt.Fprintf(out, "  Dependencies: %d\n", depCount)
			for _, ji := range jobInfos {
				gateStr := "no"
				if ji.hasGate {
					gateStr = "yes"
				}
				fmt.Fprintf(out, "  [%d] %s (gate: %s)\n", ji.number, ji.name, gateStr)
			}

			return nil
		},
	}
}
