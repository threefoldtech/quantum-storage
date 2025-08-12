package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/config"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/grid"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/zstor"
)

var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy backend ZDBs on the ThreeFold Grid",
	Long: `Deploys metadata and data ZDBs on specified or automatically selected nodes.
The command will retry failed deployments on new nodes from the specified farms until the desired count is met.`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.LoadConfig(ConfigFile)
		if err != nil {
			fmt.Printf("Error loading config: %v\n", err)
			os.Exit(1)
		}

		if len(cfg.MetaNodes) == 0 && len(cfg.Farms) == 0 {
			fmt.Println("Error: either meta_nodes or farms must be specified in config")
			os.Exit(1)
		}
		if cfg.ExpectedShards > 0 && len(cfg.DataNodes) == 0 && len(cfg.Farms) == 0 {
			fmt.Println("Error: either data_nodes or farms must be specified when expected_shards > 0")
			os.Exit(1)
		}
		if cfg.Password == "" {
			fmt.Println("Error: password is required in config")
			os.Exit(1)
		}
		if strings.ContainsAny(cfg.Password, "- ") {
			fmt.Println("Error: password cannot contain dashes or spaces")
			os.Exit(1)
		}

		metaDeployments, dataDeployments, err := grid.DeployBackends(cfg)
		if err != nil {
			fmt.Printf("Error deploying backends: %v\n", err)
			os.Exit(1)
		}

		zstorConfig, err := zstor.GenerateRemoteConfig(cfg, metaDeployments, dataDeployments)
		if err != nil {
			fmt.Printf("Error generating remote config: %v\n", err)
			os.Exit(1)
		}

		if err := os.WriteFile(ConfigOutPath, []byte(zstorConfig), 0644); err != nil {
			fmt.Printf("Error writing config file: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	deployCmd.Flags().StringVarP(&ConfigOutPath, "out", "o", "/etc/zstor.toml", "Path to write generated zstor config")
	rootCmd.AddCommand(deployCmd)
}
