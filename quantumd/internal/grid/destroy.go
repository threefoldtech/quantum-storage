package grid

import (
	"fmt"
	"os"

	"github.com/pkg/errors"
	"github.com/scottyeager/tfgrid-sdk-go/grid-client/deployer"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/config"
)

func DestroyAllBackends(cfg *config.Config) error {
	gridClient, err := NewGridClient(cfg.Network, cfg.Mnemonic, cfg.RelayURL, cfg.RMBTimeout)
	if err != nil {
		fmt.Printf("failed to create grid client: %v\n", err)
		os.Exit(1)
	}

	twinID := uint64(gridClient.TwinID)
	contractsToCancel, err := GetDeploymentContracts(&gridClient, twinID, cfg.DeploymentName)
	if err != nil {
		fmt.Printf("failed to query contracts for twin %d: %v\n", twinID, err)
		os.Exit(1)
	}
	if err := DestroyBackends(&gridClient, contractsToCancel); err != nil {
		return err
	}
	return nil
}

func DestroyBackends(gridClient *deployer.TFPluginClient, contractsToCancel []DeploymentInfo) error {
	if len(contractsToCancel) == 0 {
		fmt.Println("No deployments to destroy.")
		return nil
	}

	contractIDs := make([]uint64, 0, len(contractsToCancel))
	for _, contract := range contractsToCancel {
		contractIDs = append(contractIDs, uint64(contract.Contract.ContractID))
	}

	fmt.Printf("Destroying deployments with contract IDs %v\n", contractIDs)
	if err := gridClient.SubstrateConn.BatchCancelContract(gridClient.Identity, contractIDs); err != nil {
		return errors.Wrap(err, "failed to destroy deployments")
	}

	fmt.Println("All deployments destroyed successfully.")
	return nil
}
