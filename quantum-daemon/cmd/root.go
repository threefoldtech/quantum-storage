package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/threefoldtech/tfgrid-sdk-go/grid-client/deployer"
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
			if _, err := deployer.NewTFPluginClient(Mnemonic, deployer.WithRelayURL(relay)); err != nil {
				fmt.Printf("Invalid mnemonic or connection error: %v\n", err)
				os.Exit(1)
			}
		}
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&Mnemonic, "mnemonic", "m", os.Getenv("MNEMONIC"), "ThreeFold mnemonic for deployment (or use MNEMONIC env var)")
	rootCmd.PersistentFlags().StringVarP(&Network, "network", "n", os.Getenv("NETWORK"), "TF Grid network (dev, test, main) (or use NETWORK env var)")
	rootCmd.PersistentFlags().StringVarP(&ConfigFile, "config", "c", "", "Path to YAML config file")
}

type Config struct {
	Network    string   `yaml:"network"`
	Mnemonic   string   `yaml:"mnemonic"`
	MetaNodes  []uint32 `yaml:"meta_nodes"`
	DataNodes  []uint32 `yaml:"data_nodes"`
	ZDBPass    string   `yaml:"zdb_password"`
	MetaSizeGB int      `yaml:"meta_size_gb"`
	DataSizeGB int      `yaml:"data_size_gb"`
}

var (
	LocalMode    bool
	ServiceFiles embed.FS
	Mnemonic     string
	Network      string = func() string {
		if env := os.Getenv("NETWORK"); env != "" {
			return env
		}
		return "dev" // default to devnet
	}()
	ConfigFile string
	AppConfig  Config
)

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
