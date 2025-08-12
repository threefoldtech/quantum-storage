package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/config"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/grid"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/zstor"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initializes a full QSFS deployment, combining deploy and setup.",
	Long: `This command automates the entire process of setting up a QSFS instance.
For remote deployments, it first deploys ZDB backends on the grid and then sets up the local machine.
For local deployments (using --local), it skips the grid deployment and sets up a local test environment.
It essentially runs 'deploy' followed by 'setup'.`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.LoadConfig(ConfigFile)
		if err != nil {
			fmt.Printf("Error loading config: %v\n", err)
			os.Exit(1)
		}

		isLocal, _ := cmd.Flags().GetBool("local")
		destroy, _ := cmd.Flags().GetBool("destroy")

		if destroy {
			fmt.Println("Destroying existing deployments...")
			if err := grid.DestroyAllBackends(cfg); err != nil {
				fmt.Printf("Error destroying deployments: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("Deployments destroyed successfully.")
		}

		if !isLocal {
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
		}

		fmt.Println("Setting up QSFS components...")
		if err := SetupQSFS(isLocal); err != nil {
			fmt.Printf("Error setting up QSFS: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("QSFS setup completed successfully.")
		fmt.Println("Initialization complete.")
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().BoolP("local", "l", false, "Setup a local test environment")
	initCmd.Flags().BoolP("destroy", "d", false, "Destroy existing deployments before initializing")
	initCmd.Flags().StringVarP(&ConfigOutPath, "out", "o", "/etc/zstor.toml", "Path to write generated zstor config")
}
