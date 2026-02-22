package main

import (
	"fmt"
	"os"

	"github.com/fuzzypi/claw/internal/cli"
	"github.com/fuzzypi/claw/internal/store"
	"github.com/spf13/cobra"
)

var version = "1.0.0"

var rootCmd = &cobra.Command{
	Use:   "claw",
	Short: "Claw — orchestration engine for AOS",
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("claw v%s\n", version)
	},
}

func main() {
	s, err := store.Open("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open store: %v\n", err)
		os.Exit(1)
	}
	defer s.Close()

	pipelineCmd := &cobra.Command{Use: "pipeline", Short: "Manage pipelines"}
	pipelineCmd.AddCommand(cli.PipelineCreateCmd(s))

	jobCmd := &cobra.Command{Use: "job", Short: "Manage jobs"}
	jobCmd.AddCommand(cli.JobAddCmd(s))

	agentCmd := &cobra.Command{Use: "agent", Short: "Manage agents"}
	agentCmd.AddCommand(cli.AgentRegisterCmd(s))
	agentCmd.AddCommand(cli.AgentListCmd(s))

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(cli.RunCmd(s))
	rootCmd.AddCommand(cli.StatusCmd(s))
	rootCmd.AddCommand(pipelineCmd)
	rootCmd.AddCommand(jobCmd)
	rootCmd.AddCommand(agentCmd)
	rootCmd.AddCommand(cli.OutputCmd(s))
	rootCmd.AddCommand(cli.SummaryCmd(s))
	rootCmd.AddCommand(cli.ContextCmd(s))
	rootCmd.AddCommand(cli.GateCmd(s))
	rootCmd.AddCommand(cli.InitCmd(s))
	rootCmd.AddCommand(cli.LogCmd(s))

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
