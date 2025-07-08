package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"embed"
)

var rootCmd = &cobra.Command{
	Use:   "quantum-daemon",
	Short: "Quantum Storage Filesystem management daemon",
	Long:  `Automates the setup and management of QSFS components including zstor, zdb and zdbfs.`,
}

var (
	LocalMode bool
	ServiceFiles embed.FS
)

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
