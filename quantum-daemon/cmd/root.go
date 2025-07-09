package cmd

import (
	"fmt"
	"os"

	"embed"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "quantum-daemon",
	Short: "Quantum Storage Filesystem management daemon",
	Long:  `Automates the setup and management of QSFS components including zstor, zdb and zdbfs.`,
}

func init() {
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
