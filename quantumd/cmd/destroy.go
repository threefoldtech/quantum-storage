package cmd

import (
	"fmt"
	"os"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/grid"
)

var destroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "Destroy backend ZDBs on the ThreeFold Grid",
	Long:  `Destroys backend ZDBs on the ThreeFold Grid.`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := LoadConfig(ConfigFile)
		if err != nil {
			fmt.Printf("Error loading config: %v\n", err)
			os.Exit(1)
		}

		if err := DestroyBackends(cfg); err != nil {
			fmt.Printf("Error destroying deployments: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(destroyCmd)
}

func DestroyBackends(cfg *Config) error {
	if cfg.DeploymentName == "" {
		return errors.New("deployment_name is required in config for destroying")
	}

	network := Network
	if cfg.Network != "" {
		network = cfg.Network
	}

	gridClient, err := grid.NewGridClient(cfg.Mnemonic, network)
	if err != nil {
		return errors.Wrap(err, "failed to create grid client")
	}

	twinID := uint64(gridClient.TwinID)
	contractsToCancel, err := grid.GetDeploymentContracts(&gridClient, twinID, cfg.DeploymentName)
	if err != nil {
		return errors.Wrapf(err, "failed to query contracts for twin %d", twinID)
	}

	if len(contractsToCancel) == 0 {
		fmt.Printf("No deployments found with name starting with '%s'. Nothing to do.\n", cfg.DeploymentName)
		return nil
	}

	fmt.Printf("Found %d deployments to destroy.\n", len(contractsToCancel))

	for _, contract := range contractsToCancel {
		fmt.Printf("Destroying deployment with contract ID %d\n", contract.ContractID)
		if err := gridClient.SubstrateConn.CancelContract(gridClient.Identity, uint64(contract.ContractID)); err != nil {
			fmt.Printf("warn: failed to destroy deployment with contract ID %d: %v\n", contract.ContractID, err)
		} else {
			fmt.Printf("Destroyed deployment with contract ID %d\n", contract.ContractID)
		}
	}

	return nil
}
