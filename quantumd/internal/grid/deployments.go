package grid

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/scottyeager/tfgrid-sdk-go/grid-client/deployer"
	"github.com/scottyeager/tfgrid-sdk-go/grid-client/workloads"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/config"
	"github.com/threefoldtech/tfgrid-sdk-go/grid-proxy/pkg/types"
)

type DeploymentInfo struct {
	Contract       types.Contract
	DeploymentName string
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

func GetDeploymentContracts(grid *deployer.TFPluginClient, twinID uint64, deploymentName string) ([]DeploymentInfo, error) {
	contracts, err := GetContracts(grid, twinID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to query contracts for twin %d", twinID)
	}

	var deploymentContracts []DeploymentInfo
	for _, contractInfo := range contracts {
		name := contractInfo.DeploymentName
		expectedPrefix := fmt.Sprintf("%s_%d", deploymentName, twinID)
		if strings.HasPrefix(name, expectedPrefix) {
			parts := strings.Split(name, "_")
			if len(parts) == 4 && (parts[2] == "meta" || parts[2] == "data") {
				deploymentContracts = append(deploymentContracts, contractInfo)
			}
		}
	}
	return deploymentContracts, nil
}

func LoadExistingDeployments(gridClient *deployer.TFPluginClient, cfg *config.Config) (map[uint32]string, error) {
	existing := make(map[uint32]string)
	contracts, err := GetContracts(gridClient, uint64(gridClient.TwinID))
	if err != nil {
		return nil, err
	}

	for _, contractInfo := range contracts {
		name := contractInfo.DeploymentName
		expectedPrefix := fmt.Sprintf("%s_%d", cfg.DeploymentName, gridClient.TwinID)
		if !strings.HasPrefix(name, expectedPrefix) {
			continue
		}

		parts := strings.Split(name, "_")
		if len(parts) != 4 {
			continue
		}
		nodeType := parts[2]
		nodeID, err := strconv.ParseUint(parts[3], 10, 32)
		if err != nil {
			continue
		}

		gridClient.State.StoreContractIDs(uint32(nodeID), uint64(contractInfo.Contract.ContractID))
		// This loads the deployment into grid.State, which is important for loading ZDB details later
		if _, err := LoadZDB(gridClient, cfg, uint32(nodeID), nodeType); err != nil {
			fmt.Printf("warn: could not load deployment '%s' from grid: %v\n", name, err)
			continue
		}
		existing[uint32(nodeID)] = nodeType // Store nodeID -> type ("meta" or "data")
	}
	return existing, nil
}

// LoadZDB is a helper to load a ZDB from the grid.
func LoadZDB(gridClient *deployer.TFPluginClient, cfg *config.Config, nodeID uint32, nodeType string) (*workloads.ZDB, error) {
	twinID := uint64(gridClient.TwinID)
	name := fmt.Sprintf("%s_%d_%s_%d", cfg.DeploymentName, twinID, nodeType, nodeID)
	resZDB, err := gridClient.State.LoadZdbFromGrid(context.TODO(), nodeID, name, name)
	if err != nil {
		return nil, err
	}
	return &resZDB, nil
}
