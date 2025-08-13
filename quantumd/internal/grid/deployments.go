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

type NamedContract struct {
	Contract       types.Contract
	DeploymentName string
}

func GetContracts(grid *deployer.TFPluginClient, twinID uint64) ([]NamedContract, error) {
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

	var deploymentContracts []NamedContract
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
			deploymentContracts = append(deploymentContracts, NamedContract{
				Contract:       contract,
				DeploymentName: name,
			})
		}
	}

	return deploymentContracts, nil
}

func GetDeploymentContracts(grid *deployer.TFPluginClient, twinID uint64, deploymentName string) ([]NamedContract, error) {
	contracts, err := GetContracts(grid, twinID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to query contracts for twin %d", twinID)
	}

	var deploymentContracts []NamedContract
	for _, contractInfo := range contracts {
		name := contractInfo.DeploymentName
		deploymentNameParsed, twinIDParsed, nodeType, _, err := ParseZDBName(name)
		if err != nil {
			continue
		}
		if deploymentNameParsed == deploymentName && twinIDParsed == twinID && (nodeType == "meta" || nodeType == "data") {
			deploymentContracts = append(deploymentContracts, contractInfo)
		}
	}
	return deploymentContracts, nil
}

func LoadExistingDeployments(gridClient *deployer.TFPluginClient, cfg *config.Config) ([]workloads.Deployment, []workloads.Deployment, error) {
	contracts, err := GetContracts(gridClient, uint64(gridClient.TwinID))
	if err != nil {
		return nil, nil, err
	}

	var metaDeployments []workloads.Deployment
	var dataDeployments []workloads.Deployment

	for _, contractInfo := range contracts {
		name := contractInfo.DeploymentName
		deploymentName, twinID, nodeType, nodeID, err := ParseZDBName(name)
		if err != nil {
			fmt.Printf("warn: could not parse zdb name '%s': %v\n", name, err)
			continue
		}
		if deploymentName != cfg.DeploymentName || twinID != uint64(gridClient.TwinID) {
			fmt.Printf("warn: skipping deployment '%s' (deployment name or twin ID mismatch)\n", name)
			continue
		}

		// We must store contracts in State before trying to load deployments
		gridClient.State.StoreContractIDs(uint32(nodeID), uint64(contractInfo.Contract.ContractID))
		deployment, err := gridClient.State.LoadDeploymentFromGrid(context.TODO(), uint32(nodeID), name)

		if err != nil {
			fmt.Printf("warn: could not load deployment '%s' from grid: %v\n", name, err)
			continue
		}

		if len(deployment.Zdbs) == 0 {
			fmt.Printf("warn: deployment '%s' has no ZDBs\n", name)
			continue
		}

		if len(deployment.Zdbs) != 1 {
			fmt.Printf("warn: deployment '%s' has more than one ZDB\n", name)
			continue
		}

		if nodeType == "meta" {
			metaDeployments = append(metaDeployments, deployment)
			fmt.Printf("Found metadata ZDB '%s' on node %d\n", name, nodeID)
		} else if nodeType == "data" {
			dataDeployments = append(dataDeployments, deployment)
			fmt.Printf("Found data ZDB '%s' on node %d\n", name, nodeID)
		}
	}
	return metaDeployments, dataDeployments, nil
}

// LoadZDB is a helper to load a ZDB from the grid.
func LoadZDB(gridClient *deployer.TFPluginClient, cfg *config.Config, nodeID uint32, nodeType string) (*workloads.ZDB, error) {
	twinID := uint64(gridClient.TwinID)
	name := MakeZDBName(cfg.DeploymentName, twinID, nodeType, nodeID)
	resZDB, err := gridClient.State.LoadZdbFromGrid(context.TODO(), nodeID, name, name)
	if err != nil {
		return nil, err
	}
	return &resZDB, nil
}

// MakeZDBName creates a zdb name string from the given components.
func MakeZDBName(deploymentName string, twinID uint64, nodeType string, nodeID uint32) string {
	return fmt.Sprintf("%s_%d_%s_%d", deploymentName, twinID, nodeType, nodeID)
}

// ParseZDBName parses a zdb name string into its components.
// Returns the deployment name, twin ID, node type, node ID, and an error if parsing fails.
func ParseZDBName(name string) (string, uint64, string, uint32, error) {
	parts := strings.Split(name, "_")
	if len(parts) != 4 {
		return "", 0, "", 0, fmt.Errorf("invalid zdb name format: expected 4 parts separated by underscores")
	}

	deploymentName := parts[0]
	twinID, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return "", 0, "", 0, fmt.Errorf("invalid twin ID in zdb name: %v", err)
	}

	nodeType := parts[2]
	nodeID, err := strconv.ParseUint(parts[3], 10, 32)
	if err != nil {
		return "", 0, "", 0, fmt.Errorf("invalid node ID in zdb name: %v", err)
	}

	return deploymentName, twinID, nodeType, uint32(nodeID), nil
}
