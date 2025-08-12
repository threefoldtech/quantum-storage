package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/config"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/grid"
)

var force bool

var destroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "Destroy backend ZDBs on the ThreeFold Grid",
	Long:  `Destroys backend ZDBs on the ThreeFold Grid.`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.LoadConfig(ConfigFile)
		if err != nil {
			fmt.Printf("Error loading config: %v\n", err)
			os.Exit(1)
		}

		gridClient, err := grid.NewGridClient(cfg.Network, cfg.Mnemonic, cfg.RelayURL, cfg.RMBTimeout)

		if err != nil {
			fmt.Printf("failed to create grid client: %v\n", err)
			os.Exit(1)
		}

		twinID := uint64(gridClient.TwinID)
		contractsToCancel, err := grid.GetDeploymentContracts(&gridClient, twinID, cfg.DeploymentName)
		if err != nil {
			fmt.Printf("failed to query contracts for twin %d: %v\n", twinID, err)
			os.Exit(1)
		}

		if len(contractsToCancel) == 0 {
			fmt.Printf("No deployments found with name starting with '%s'. Nothing to do.\n", cfg.DeploymentName)
			return
		}

		fmt.Printf("Found %d deployments to destroy:\n", len(contractsToCancel))
		for _, contract := range contractsToCancel {
			fmt.Printf("  - Name: %s, Contract ID: %d\n", contract.DeploymentName, contract.Contract.ContractID)
		}

		if !force {
			fmt.Print("Are you sure you want to destroy all deployments? (y/n) ")
			reader := bufio.NewReader(os.Stdin)
			input, _ := reader.ReadString('\n')
			if strings.TrimSpace(strings.ToLower(input)) != "y" {
				fmt.Println("Destroy operation cancelled.")
				os.Exit(0)
			}
		}

		if err := grid.DestroyBackends(&gridClient, contractsToCancel); err != nil {
			fmt.Printf("Error destroying deployments: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(destroyCmd)
	destroyCmd.Flags().BoolVarP(&force, "force", "f", false, "Force destruction without confirmation")
}
