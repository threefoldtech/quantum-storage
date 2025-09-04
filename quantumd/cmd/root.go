package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "quantumd",
	Short: "Quantum Storage Filesystem management daemon",
	Long:  `Automates the setup and management of QSFS components including zstor, zdb and zdbfs.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// If no command is specified, print help
		return cmd.Help()
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&ConfigFile, "config", "c", "/etc/quantumd.yaml", "Path to YAML config file")
	rootCmd.PersistentFlags().Bool("version", false, "Print the version number of quantumd")

	// Add version flag handler
	rootCmd.PreRun = func(cmd *cobra.Command, args []string) {
		if showVersion, _ := cmd.Flags().GetBool("version"); showVersion {
			fmt.Printf("quantumd version %s\n", Version)
			if Commit != "" {
				fmt.Printf("commit: %s\n", Commit)
			}
			if Date != "" {
				fmt.Printf("built at: %s\n", Date)
			}
			os.Exit(0)
		}
	}
}

var (
	ConfigFile string
	// Version information will be set during build
	Version = "dev"
	Commit  = ""
	Date    = ""
)

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
