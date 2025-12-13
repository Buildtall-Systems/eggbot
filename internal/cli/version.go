package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var (
	version   = "dev"
	commit    = "none"
	date      = "unknown"
	builtBy   = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("eggbot %s\n", version)
		fmt.Printf("  commit: %s\n", commit)
		fmt.Printf("  built: %s by %s\n", date, builtBy)
		fmt.Printf("  go: %s\n", runtime.Version())
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
