package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initializes a full QSFS deployment, combining deploy and setup.",
	Long: `This command automates the entire process of setting up a QSFS instance.
For remote deployments, it first deploys ZDB backends on the grid and then sets up the local machine.
For local deployments (using --local), it skips the grid deployment and sets up a local test environment.
It essentially runs 'deploy' followed by 'setup'.`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := LoadConfig(ConfigFile)
		if err != nil {
			fmt.Printf("Error loading config: %v\n", err)
			os.Exit(1)
		}

		isLocal, _ := cmd.Flags().GetBool("local")
		destroy, _ := cmd.Flags().GetBool("destroy")

		if destroy {
			fmt.Println("Destroying existing deployments...")
			if err := DestroyBackends(cfg); err != nil {
				fmt.Printf("Error destroying deployments: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("Deployments destroyed successfully.")
			return
		}

		if !isLocal {
			fmt.Println("Deploying backends on the grid...")
			if err := DeployBackends(cfg); err != nil {
				fmt.Printf("Error deploying backends: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("Backends deployed successfully.")
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
