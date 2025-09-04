package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of quantumd",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("quantumd version %s\n", Version)
		if Commit != "" {
			fmt.Printf("commit: %s\n", Commit)
		}
		if Date != "" {
			fmt.Printf("built at: %s\n", Date)
		}
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
