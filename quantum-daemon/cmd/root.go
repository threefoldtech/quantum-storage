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
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Validate mnemonic if provided
		if Mnemonic != "" {
			relay := "wss://relay.grid.tf"
			if Network != "main" {
				relay = fmt.Sprintf("wss://relay.%s.grid.tf", Network)
			}
			if _, err := sdk.NewSubstrateExt(relay, Mnemonic); err != nil {
				fmt.Printf("Invalid mnemonic: %v\n", err)
				os.Exit(1)
			}
		}
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&Mnemonic, "mnemonic", "m", "", "ThreeFold mnemonic for deployment")
	rootCmd.PersistentFlags().StringVarP(&Network, "network", "n", "dev", "TF Grid network (dev, test, main)")
}

var (
	LocalMode bool
	ServiceFiles embed.FS
	Mnemonic string
	Network string = "dev" // default to devnet
)

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
