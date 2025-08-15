package grid

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/scottyeager/tfgrid-sdk-go/grid-client/deployer"
	"github.com/scottyeager/tfgrid-sdk-go/grid-client/workloads"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/config"
)

// Zstor has a hardcoded metadata node count of 4
const metaNodeCount = 4

func DeployBackends(gridClient deployer.TFPluginClient, cfg *config.Config) ([]workloads.Deployment, []workloads.Deployment, error) {
	if cfg.MetaSizeGb <= 0 {
		return nil, nil, fmt.Errorf("meta_size must be greater than 0")
	}
	if cfg.DataSizeGb <= 0 {
		return nil, nil, fmt.Errorf("data_size or total_storage_size must be set to a value greater than 0")
	}

	deploymentDeployer := deployer.NewDeploymentDeployer(&gridClient)

	// Load existing deployments
	existingMetaDeployments, existingDataDeployments, err := LoadExistingDeployments(&gridClient, cfg)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to load existing deployments")
	}
	fmt.Printf("Found %d existing deployments.\n", len(existingMetaDeployments)+len(existingDataDeployments))

	// Extract node IDs from existing deployments
	var existingMetaNodes, existingDataNodes []uint32
	for _, d := range existingMetaDeployments {
		existingMetaNodes = append(existingMetaNodes, d.NodeID)
	}
	for _, d := range existingDataDeployments {
		existingDataNodes = append(existingDataNodes, d.NodeID)
	}

	// Node pool for automatic selection
	nodePool := NewNodePool(cfg, &gridClient, existingMetaNodes, existingDataNodes)

	requiredMetaCount := metaNodeCount - len(existingMetaNodes)
	// Deploy metadata ZDBs
	metaDeployments, err := deployInBatches(
		&deploymentDeployer, &gridClient, cfg, "meta", cfg.MetaSizeGb, workloads.ZDBModeUser, requiredMetaCount,
		cfg.MetaNodes, nil, nodePool,
	)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to deploy metadata ZDBs")
	}

	// After successful metadata deployment, get the node IDs to use as preferred nodes for data deployment.
	metaNodes := []uint32{}
	for _, zdb := range metaDeployments {
		parts := strings.Split(zdb.Name, "_")
		if len(parts) > 0 {
			nodeID, err := strconv.ParseUint(parts[len(parts)-1], 10, 32)
			if err == nil {
				metaNodes = append(metaNodes, uint32(nodeID))
			}
		}
	}

	requiredDataCount := cfg.ExpectedShards - len(existingDataNodes)
	// Deploy data ZDBs, preferring metadata nodes after any manually specified data nodes.
	dataDeployments, err := deployInBatches(
		&deploymentDeployer, &gridClient, cfg, "data", cfg.DataSizeGb, workloads.ZDBModeSeq, requiredDataCount,
		cfg.DataNodes, metaNodes, nodePool,
	)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to deploy data ZDBs")
	}

	allMetaDeployments := append(existingMetaDeployments, metaDeployments...)
	allDataDeployments := append(existingDataDeployments, dataDeployments...)

	return allMetaDeployments, allDataDeployments, nil
}

// deployInBatches handles the entire lifecycle of deploying a group of ZDBs,
// including retries on failure.
func deployInBatches(
	deploymentDeployer *deployer.DeploymentDeployer,
	gridClient *deployer.TFPluginClient,
	cfg *config.Config,
	nodeType string,
	sizeGB int,
	mode string,
	requiredCount int,
	manualNodes []uint32,
	preferredNodes []uint32,
	pool *NodePool,
) ([]workloads.Deployment, error) {

	twinID := uint64(gridClient.TwinID)
	successfulDeployments := []workloads.Deployment{}
	nodesToDeploy := []uint32{}

	// Build the list of nodes to deploy, respecting priority.
	processedForDeployList := make(map[uint32]bool)
	addNode := func(nodeID uint32) {
		if _, ok := processedForDeployList[nodeID]; ok {
			return // already added
		}
		// Nodes may have both a data and a meta zdb.
		if (pool.IsDataNode(nodeID) && nodeType == "meta") || (pool.IsMetaNode(nodeID) && nodeType == "data") {
			nodesToDeploy = append(nodesToDeploy, nodeID)
			processedForDeployList[nodeID] = true
		}
	}

	// Prioritize manual nodes
	for _, nodeID := range manualNodes {
		addNode(nodeID)
	}

	// Then add preferred nodes
	for _, nodeID := range preferredNodes {
		addNode(nodeID)
	}

	// Loop until we have enough deployments
	retries := 0
	for len(successfulDeployments) < requiredCount {
		if retries >= cfg.MaxDeploymentRetries {
			return nil, fmt.Errorf("failed to deploy required number of %s ZDBs after %d retries", nodeType, cfg.MaxDeploymentRetries)
		}
		needed := requiredCount - len(successfulDeployments)

		// Get candidate nodes for the batch
		var candidates []uint32
		if len(nodesToDeploy) > 0 {
			// Prioritize manually specified nodes
			if needed > len(nodesToDeploy) {
				candidates = nodesToDeploy
			} else {
				candidates = nodesToDeploy[:needed]
			}
			nodesToDeploy = nodesToDeploy[len(candidates):]
		} else {
			// If no manual nodes left, get new ones from the pool
			fmt.Printf("Needing %d more %s nodes, searching farms...\n", needed, nodeType)
			newCandidates, err := pool.Get(needed)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to get candidate nodes for %s deployment", nodeType)
			}
			candidates = newCandidates
		}

		if len(candidates) == 0 {
			return nil, fmt.Errorf("could not find any new candidate nodes for %s deployment", nodeType)
		}

		// Prepare batch deployment
		deployments := []*workloads.Deployment{}
		nodeIDsForBatch := []uint32{}
		for _, nodeID := range candidates {
			pool.MarkUsed(nodeID, nodeType) // Mark as used immediately
			nodeIDsForBatch = append(nodeIDsForBatch, nodeID)
			name := MakeZDBName(cfg.DeploymentName, twinID, nodeType, nodeID)
			zdb := workloads.ZDB{
				Name:        name,
				Password:    cfg.Password,
				Public:      false,
				SizeGB:      uint64(sizeGB),
				Description: fmt.Sprintf("QSFS %s namespace", nodeType),
				Mode:        mode,
			}
			dl := workloads.NewDeployment(name, nodeID, "", nil, "", nil, []workloads.ZDB{zdb}, nil, nil, nil, nil)
			deployments = append(deployments, &dl)
		}

		// Deploy the batch
		fmt.Printf("Attempting to deploy a batch of %d %s ZDBs of size %dGB on nodes %v...\n", len(deployments), nodeType, sizeGB, nodeIDsForBatch)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		err := deploymentDeployer.BatchDeploy(ctx, deployments)

		// Process batch results
		if err != nil {
			fmt.Printf("warn: batch deployment failed: %v. Analyzing individual deployments...\n", err)
		}

		// Check which deployments succeeded and load them
		for _, dl := range deployments {
			// After BatchDeploy, contract ID should be updated in the deployment object.
			if dl.ContractID == 0 {
				fmt.Printf("warn: deployment on node %d failed to get a contract ID.\n", dl.NodeID)
				continue
			}

			successfulDeployments = append(successfulDeployments, *dl)
			fmt.Printf("Successfully deployed and loaded %s ZDB of size %dGB on node %d\n", nodeType, sizeGB, dl.NodeID)
		}

		if len(successfulDeployments) < requiredCount {
			retries++
			fmt.Printf("Deployed %d/%d %s ZDBs, retrying for the remaining ones (attempt %d/%d).\n", len(successfulDeployments), requiredCount, nodeType, retries, cfg.MaxDeploymentRetries)
			time.Sleep(2 * time.Second) // Brief pause before next batch
		}
	}

	fmt.Printf("Successfully deployed all %d %s ZDBs.\n", requiredCount, nodeType)
	return successfulDeployments, nil
}
