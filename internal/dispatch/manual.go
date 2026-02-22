package dispatch

import (
	"fmt"
	"io"

	"github.com/fuzzypi/claw/internal/gates"
	"github.com/fuzzypi/claw/internal/store"
)

// ExecuteManual prints the prompt (with context) and reads paste-back from the
// provided stdin/stdout streams, making it testable without real terminal I/O.
func ExecuteManual(s *store.Store, job *store.Job, agent *store.Agent, stdin io.Reader, stdout io.Writer) error {
	fmt.Fprintf(stdout, "--- JOB: %s ---\n", job.Name)

	// Inject accumulated context
	entries, err := s.ListContextByPipeline(job.PipelineID)
	if err == nil && len(entries) > 0 {
		contents := make([]string, len(entries))
		for i, e := range entries {
			contents[i] = e.Content
		}
		header := gates.BuildContextHeader(contents)
		fmt.Fprintln(stdout, header)
	}

	fmt.Fprintln(stdout, job.Prompt)
	fmt.Fprintln(stdout, "\n--- Paste agent output below (Ctrl+D to end) ---")

	data, err := io.ReadAll(stdin)
	if err != nil {
		return fmt.Errorf("reading manual input: %w", err)
	}

	output := truncateOutput(string(data))

	if err := s.SetJobOutput(job.ID, output, 0); err != nil {
		return err
	}

	return nil
}
