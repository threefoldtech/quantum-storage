package grid

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/threefoldtech/tfgrid-sdk-go/grid-client/deployer"
	"github.com/threefoldtech/tfgrid-sdk-go/grid-proxy/pkg/types"
)

type DeploymentInfo struct {
	Contract       types.Contract
	DeploymentName string
}

func NewGridClient(mnemonic, network string) (deployer.TFPluginClient, error) {
	relay := "wss://relay.grid.tf"
	if network != "main" {
		relay = fmt.Sprintf("wss://relay.%s.grid.tf", network)
	}

	return deployer.NewTFPluginClient(mnemonic,
		deployer.WithRelayURL(relay),
		deployer.WithNetwork(network),
		deployer.WithRMBTimeout(100),
	)
}

func GetContracts(grid *deployer.TFPluginClient, twinID uint64) ([]DeploymentInfo, error) {
	allContracts := make([]types.Contract, 0)
	page := uint64(1)
	const pageSize = 100

	filter := types.ContractFilter{
		TwinID: &twinID,
		State:  []string{"Created"},
	}

	for {
		limit := types.Limit{
			Size: pageSize,
			Page: page,
		}

		contracts, _, err := grid.GridProxyClient.Contracts(context.Background(), filter, limit)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to query contracts page %d", page)
		}

		allContracts = append(allContracts, contracts...)

		if len(contracts) < pageSize {
			break
		}

		page++
	}

	var deploymentContracts []DeploymentInfo
	for _, contract := range allContracts {
		if contract.Type != "node" {
			continue
		}

		var name string
		var deploymentData string

		// Handle both possible types for contract.Details
		if details, ok := contract.Details.(types.NodeContractDetails); ok {
			deploymentData = details.DeploymentData
		} else if details, ok := contract.Details.(map[string]interface{}); ok {
			if dd, ok := details["deployment_data"].(string); ok {
				deploymentData = dd
			}
		}

		if deploymentData != "" {
			var data struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal([]byte(deploymentData), &data); err == nil {
				name = data.Name
			}
		}

		if name != "" {
			deploymentContracts = append(deploymentContracts, DeploymentInfo{
				Contract:       contract,
				DeploymentName: name,
			})
		}
	}

	return deploymentContracts, nil
}

func GetDeploymentContracts(grid *deployer.TFPluginClient, twinID uint64, deploymentName string) ([]types.Contract, error) {
	contracts, err := GetContracts(grid, twinID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to query contracts for twin %d", twinID)
	}

	var contractsToCancel []types.Contract
	for _, contractInfo := range contracts {
		name := contractInfo.DeploymentName
		expectedPrefix := fmt.Sprintf("%s_%d", deploymentName, twinID)
		if strings.HasPrefix(name, expectedPrefix) {
			parts := strings.Split(name, "_")
			if len(parts) == 4 && (parts[2] == "meta" || parts[2] == "data") {
				contractsToCancel = append(contractsToCancel, contractInfo.Contract)
			}
		}
	}
	return contractsToCancel, nil
}

func GetNodesFromFarms(grid *deployer.TFPluginClient, farmIDs []uint64, hru, sru uint64) ([]uint32, error) {
	rentedFalse := false
	filter := types.NodeFilter{
		FarmIDs: farmIDs,
		FreeSRU: &sru,
		FreeHRU: &hru,
		Rented:  &rentedFalse,
		Status:  []string{"up"},
	}

	nodes, _, err := grid.GridProxyClient.Nodes(context.Background(), filter, types.Limit{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to query nodes from grid proxy")
	}

	var nodeIDs []uint32
	for _, node := range nodes {
		nodeIDs = append(nodeIDs, uint32(node.NodeID))
	}

	return nodeIDs, nil
}
