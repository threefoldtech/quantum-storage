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
		fmt.Printf("quantumd version %s\n", version)
		if commit != "" {
			fmt.Printf("commit: %s\n", commit)
		}
		if date != "" {
			fmt.Printf("built at: %s\n", date)
		}
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
