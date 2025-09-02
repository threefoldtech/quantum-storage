package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

const (
	defaultRetryInterval = 10 * time.Minute
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
	rootCmd.PersistentFlags().BoolVarP(&localMode, "local", "l", false, "Enable local mode")
	rootCmd.PersistentFlags().Bool("version", false, "Print the version number of quantumd")

	// Add version flag handler
	rootCmd.PreRun = func(cmd *cobra.Command, args []string) {
		if showVersion, _ := cmd.Flags().GetBool("version"); showVersion {
			fmt.Printf("quantumd version %s\n", version)
			if commit != "" {
				fmt.Printf("commit: %s\n", commit)
			}
			if date != "" {
				fmt.Printf("built at: %s\n", date)
			}
			os.Exit(0)
		}
	}
}

var (
	ConfigFile string
	// Version information will be set during build
	version = "dev"
	commit  = ""
	date    = ""
)

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
