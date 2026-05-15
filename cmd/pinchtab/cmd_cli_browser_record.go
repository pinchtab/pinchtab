package main

import (
	browseractions "github.com/pinchtab/pinchtab/internal/cli/actions"
	"github.com/spf13/cobra"
)

var recordCmd = &cobra.Command{
	Use:   "record",
	Short: "Record browser activity to video",
}

var recordStartCmd = &cobra.Command{
	Use:   "start <file>",
	Short: "Start recording (.webm, .mp4, .gif)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.RecordStart(rt.client, rt.base, rt.token, cmd, args)
		})
	},
}

var recordStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop recording and save",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.RecordStop(rt.client, rt.base, rt.token)
		})
	},
}

var recordStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check recording status",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.RecordStatus(rt.client, rt.base, rt.token)
		})
	},
}
