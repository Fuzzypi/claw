package dispatch

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"

	"github.com/fuzzypi/claw/internal/gates"
	"github.com/fuzzypi/claw/internal/store"
)

const maxOutputBytes = 1_048_576
const keepBytes = 262_144

// ExecuteShell runs the agent's command with the job prompt piped via stdin.
// Accumulated pipeline context is prepended to the prompt.
func ExecuteShell(s *store.Store, job *store.Job, agent *store.Agent) error {
	if agent.Command == nil {
		return fmt.Errorf("agent %q has no command configured", agent.Name)
	}

	timeoutSecs := agent.TimeoutSecs
	if timeoutSecs <= 0 {
		timeoutSecs = 600
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSecs)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, *agent.Command, agent.Args...)

	if agent.Cwd != nil && *agent.Cwd != "" {
		cmd.Dir = *agent.Cwd
	} else {
		p, err := s.GetPipeline(job.PipelineID)
		if err == nil && p.ProjectDir != "" {
			cmd.Dir = p.ProjectDir
		}
	}

	// Build full prompt with context injection
	fullPrompt := buildFullPrompt(s, job)
	cmd.Stdin = strings.NewReader(fullPrompt)

	var combined bytes.Buffer
	cmd.Stdout = &combined
	cmd.Stderr = &combined

	err := cmd.Run()

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	output := combined.String()
	output, rtkStats := filterThroughRTK(output)
	output = truncateOutput(output)

	if rtkStats != "" {
		log.Printf("[claw] %s", rtkStats)
		if logErr := s.LogActivity(&job.PipelineID, &job.ID, nil, "rtk_filtered", rtkStats); logErr != nil {
			log.Printf("[claw] activity log error: %v", logErr)
		}
	}

	if storeErr := s.SetJobOutput(job.ID, output, exitCode); storeErr != nil {
		return storeErr
	}

	if exitCode != 0 {
		return fmt.Errorf("command exited with code %d", exitCode)
	}
	return nil
}

func buildFullPrompt(s *store.Store, job *store.Job) string {
	entries, err := s.ListContextByPipeline(job.PipelineID)
	if err != nil || len(entries) == 0 {
		return job.Prompt
	}

	contents := make([]string, len(entries))
	for i, e := range entries {
		contents[i] = e.Content
	}

	header := gates.BuildContextHeader(contents)
	return header + "\n" + job.Prompt
}

func filterThroughRTK(output string) (string, string) {
	_, err := exec.LookPath("rtk")
	if err != nil {
		return output, ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "rtk", "--stats")
	cmd.Stdin = strings.NewReader(output)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		log.Printf("[claw] rtk filter failed: %v", err)
		return output, ""
	}

	return stdout.String(), strings.TrimSpace(stderr.String())
}

func truncateOutput(output string) string {
	if len(output) <= maxOutputBytes {
		return output
	}
	dropped := len(output) - 2*keepBytes
	return output[:keepBytes] +
		fmt.Sprintf("\n[... truncated %d bytes ...]\n", dropped) +
		output[len(output)-keepBytes:]
}
