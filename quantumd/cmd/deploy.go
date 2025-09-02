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
		queryMode, _ := cmd.Flags().GetBool("query")

		cfg, err := config.LoadConfig(ConfigFile)
		if err != nil {
			fmt.Printf("Error loading config: %v\n", err)
			os.Exit(1)
		}

		gridClient, err := grid.NewGridClient(cfg.Network, cfg.Mnemonic, cfg.RelayURL, cfg.RMBTimeout)

		if err != nil {
			fmt.Printf("Error creating grid client: %v\n", err)
			os.Exit(1)
		}

		if queryMode {
			metaDeployments, dataDeployments, err := grid.LoadExistingDeployments(&gridClient, cfg)
			if err != nil {
				fmt.Printf("Error querying deployments: %v\n", err)
				os.Exit(1)
			}

			fmt.Println("Metadata deployments:")
			for _, deployment := range metaDeployments {
				fmt.Printf("  Contract ID: %d, Node ID: %d, ZDB Info: %s\n",
					deployment.ContractID, deployment.NodeID, deployment.Zdbs[0])
			}

			fmt.Println("Data deployments:")
			for _, deployment := range dataDeployments {
				fmt.Printf("  Contract ID: %d, Node ID: %d, ZDB Info: %s\n",
					deployment.ContractID, deployment.NodeID, deployment.Zdbs[0])
			}
			return
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

		metaDeployments, dataDeployments, err := grid.DeployBackends(gridClient, cfg)
		if err != nil {
			fmt.Printf("Error deploying backends: %v\n", err)
			os.Exit(1)
		}

		zstorConfig, err := zstor.GenerateRemoteConfig(cfg, metaDeployments, dataDeployments)
		if err != nil {
			fmt.Printf("Error generating remote config: %v\n", err)
			os.Exit(1)
		}

		if err := os.WriteFile(cfg.ZstorConfigPath, []byte(zstorConfig), 0644); err != nil {
			fmt.Printf("Error writing config file: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	deployCmd.Flags().BoolP("query", "q", false, "Query existing deployments instead of deploying new ones")
	rootCmd.AddCommand(deployCmd)
}
