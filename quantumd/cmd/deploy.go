package cmd

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/scottyeager/tfgrid-sdk-go/grid-client/deployer"
	"github.com/scottyeager/tfgrid-sdk-go/grid-client/workloads"
	"github.com/spf13/cobra"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/config"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/grid"
	bip39 "github.com/tyler-smith/go-bip39"
)

func mapIPs(ips []string) map[string]string {
	mapped := make(map[string]string)
	for _, ip := range ips {
		parts := strings.Split(ip, ":")
		if len(parts) == 0 {
			continue
		}
		firstPart := parts[0]
		// Convert the first part to a hex number for range checking
		hexValue, err := strconv.ParseInt(firstPart, 16, 64)
		if err != nil {
			continue
		}
		// Check if it falls into specific ranges
		if 0x2000 <= hexValue && hexValue <= 0x3FFF {
			mapped["ipv6"] = ip
		} else if 0x200 <= hexValue && hexValue <= 0x3FF {
			mapped["ygg"] = ip
		} else if 0x400 <= hexValue && hexValue <= 0x5FF {
			mapped["mycelium"] = ip
		}
	}
	return mapped
}

var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy backend ZDBs on the ThreeFold Grid",
	Long: `Deploys metadata and data ZDBs on specified or automatically selected nodes.
The command will retry failed deployments on new nodes from the specified farms until the desired count is met.`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := LoadConfig(ConfigFile)
		if err != nil {
			fmt.Printf("Error loading config: %v\n", err)
			os.Exit(1)
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

		if err := DeployBackends(cfg); err != nil {
			fmt.Printf("Error deploying backends: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	deployCmd.Flags().StringVarP(&ConfigOutPath, "out", "o", "/etc/zstor.toml", "Path to write generated zstor config")
	rootCmd.AddCommand(deployCmd)
}

func DeployBackends(cfg *config.Config) error {
	if cfg.MetaSizeGb <= 0 {
		return fmt.Errorf("meta_size must be greater than 0")
	}
	if cfg.DataSizeGb <= 0 {
		return fmt.Errorf("data_size or total_storage_size must be set to a value greater than 0")
	}

	gridClient, err := grid.NewGridClient(cfg.Network, cfg.Mnemonic, cfg.RelayURL, cfg.RMBTimeout)

	if err != nil {
		return errors.Wrap(err, "failed to create grid client")
	}

	deploymentDeployer := deployer.NewDeploymentDeployer(&gridClient)

	// Load existing deployments
	existingDeployments, err := loadExistingDeployments(&gridClient, cfg)
	if err != nil {
		return errors.Wrap(err, "failed to load existing deployments")
	}
	fmt.Printf("Found %d existing deployments.\n", len(existingDeployments))

	// Node pool for automatic selection
	nodePool := grid.NewNodePool(cfg, &gridClient, existingDeployments)

	// Deploy metadata ZDBs
	metaDeployments, err := deployInBatches(
		&deploymentDeployer, &gridClient, cfg, "meta", cfg.MetaSizeGb, workloads.ZDBModeUser, 4,
		cfg.MetaNodes, nil, existingDeployments, nodePool,
	)
	if err != nil {
		return errors.Wrap(err, "failed to deploy metadata ZDBs")
	}

	// After successful metadata deployment, get the node IDs to use as preferred nodes for data deployment.
	metaNodeIDs := []uint32{}
	for _, zdb := range metaDeployments {
		parts := strings.Split(zdb.Name, "_")
		if len(parts) > 0 {
			nodeID, err := strconv.ParseUint(parts[len(parts)-1], 10, 32)
			if err == nil {
				metaNodeIDs = append(metaNodeIDs, uint32(nodeID))
			}
		}
	}

	// Deploy data ZDBs, preferring metadata nodes after any manually specified data nodes.
	dataDeployments, err := deployInBatches(
		&deploymentDeployer, &gridClient, cfg, "data", cfg.DataSizeGb, workloads.ZDBModeSeq, cfg.ExpectedShards,
		cfg.DataNodes, metaNodeIDs, existingDeployments, nodePool,
	)
	if err != nil {
		return errors.Wrap(err, "failed to deploy data ZDBs")
	}

	// Generate config
	return generateRemoteConfig(cfg, metaDeployments, dataDeployments)
}

// Helper to load existing deployments from the grid
func loadExistingDeployments(gridClient *deployer.TFPluginClient, cfg *config.Config) (map[uint32]string, error) {
	existing := make(map[uint32]string)
	contracts, err := grid.GetContracts(gridClient, uint64(gridClient.TwinID))
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
		if _, err := loadZDB(gridClient, cfg, uint32(nodeID), nodeType); err != nil {
			fmt.Printf("warn: could not load deployment '%s' from grid: %v\n", name, err)
			continue
		}
		existing[uint32(nodeID)] = nodeType // Store nodeID -> type ("meta" or "data")
	}
	return existing, nil
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
	existing map[uint32]string,
	pool *grid.NodePool,
) ([]*workloads.ZDB, error) {

	twinID := uint64(gridClient.TwinID)
	successfulDeployments := []*workloads.ZDB{}
	nodesToDeploy := []uint32{}

	// First, account for ALL existing deployments of the correct type.
	fmt.Printf("Checking for any existing '%s' deployments...\n", nodeType)
	for nodeID, t := range existing {
		if t == nodeType {
			if zdb, err := loadZDB(gridClient, cfg, nodeID, nodeType); err == nil {
				fmt.Printf("Found existing %s deployment of size %dGB on node %d.\n", nodeType, zdb.SizeGB, nodeID)
				successfulDeployments = append(successfulDeployments, zdb)
				pool.MarkUsed(nodeID, nodeType) // Mark node as used in the pool
			} else {
				fmt.Printf("warn: could not load existing zdb from node %d: %v\n", nodeID, err)
			}
		}
	}
	fmt.Printf("Found and loaded %d existing '%s' deployments.\n", len(successfulDeployments), nodeType)

	// Build the list of nodes to deploy, respecting priority.
	processedForDeployList := make(map[uint32]bool)
	addNode := func(nodeID uint32) {
		if _, ok := processedForDeployList[nodeID]; ok {
			return // already added
		}
		// Allow deploying a 'data' zdb on a node that has a 'meta' zdb.
		t, isUsed := pool.Used[nodeID]
		if !isUsed || (nodeType == "data" && t == "meta") {
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
		deploymentConfigs := []*workloads.Deployment{}
		nodeIDsForBatch := []uint32{}
		for _, nodeID := range candidates {
			pool.MarkUsed(nodeID, nodeType) // Mark as used immediately
			nodeIDsForBatch = append(nodeIDsForBatch, nodeID)
			name := fmt.Sprintf("%s_%d_%s_%d", cfg.DeploymentName, twinID, nodeType, nodeID)
			zdb := workloads.ZDB{
				Name:        name,
				Password:    cfg.Password,
				Public:      false,
				SizeGB:      uint64(sizeGB),
				Description: fmt.Sprintf("QSFS %s namespace", nodeType),
				Mode:        mode,
			}
			dl := workloads.NewDeployment(name, nodeID, "", nil, "", nil, []workloads.ZDB{zdb}, nil, nil, nil, nil)
			deploymentConfigs = append(deploymentConfigs, &dl)
		}

		// Deploy the batch
		fmt.Printf("Attempting to deploy a batch of %d %s ZDBs of size %dGB on nodes %v...\n", len(deploymentConfigs), nodeType, sizeGB, nodeIDsForBatch)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		err := deploymentDeployer.BatchDeploy(ctx, deploymentConfigs)

		// Process batch results
		if err != nil {
			fmt.Printf("warn: batch deployment failed: %v. Analyzing individual deployments...\n", err)
		}

		// Check which deployments succeeded and load them
		for _, dl := range deploymentConfigs {
			// After BatchDeploy, contract ID should be updated in the deployment object.
			if dl.ContractID == 0 {
				fmt.Printf("warn: deployment on node %d failed to get a contract ID.\n", dl.NodeID)
				continue
			}
			resZDB, loadErr := gridClient.State.LoadZdbFromGrid(context.TODO(), dl.NodeID, dl.Name, dl.Zdbs[0].Name)
			if loadErr != nil {
				fmt.Printf("warn: deployment on node %d failed to load after batch: %v\n", dl.NodeID, loadErr)
				continue
			}
			successfulDeployments = append(successfulDeployments, &resZDB)
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

// loadZDB is a helper to load a ZDB from the grid.
func loadZDB(gridClient *deployer.TFPluginClient, cfg *config.Config, nodeID uint32, nodeType string) (*workloads.ZDB, error) {
	twinID := uint64(gridClient.TwinID)
	name := fmt.Sprintf("%s_%d_%s_%d", cfg.DeploymentName, twinID, nodeType, nodeID)
	resZDB, err := gridClient.State.LoadZdbFromGrid(context.TODO(), nodeID, name, name)
	if err != nil {
		return nil, err
	}
	return &resZDB, nil
}

func generateRemoteConfig(cfg *config.Config, meta, data []*workloads.ZDB) error {
	key, err := keyFromMnemonic(cfg.Mnemonic, cfg.Password)
	if err != nil {
		return errors.Wrap(err, "failed to generate key from mnemonic")
	}

	size, err := parseSize(cfg.ZdbDataSize)
	if err != nil {
		return errors.Wrap(err, "failed to parse zdb_data_size")
	}
	zdbDataSizeMb := size / (1024 * 1024)

	var configBuilder strings.Builder
	configBuilder.WriteString(fmt.Sprintf(`minimal_shards = %d
expected_shards = %d
redundant_groups = 0
redundant_nodes = 0
root = "/"
zdbfs_mountpoint = "%s"
socket = "/tmp/zstor.sock"
prometheus_port = 9200
zdb_data_dir_path = "%s/data/zdbfs-data/"
max_zdb_data_dir_size = %d

[compression]
algorithm = "snappy"

[meta]
type = "zdb"

[meta.config]
prefix = "zstor-meta"

[encryption]
algorithm = "AES"
key = "%s"

[meta.config.encryption]
algorithm = "AES"
key = "%s"`, cfg.MinShards, cfg.ExpectedShards, cfg.QsfsMountpoint, cfg.ZdbRootPath, zdbDataSizeMb, key, key))

	// Add meta backends
	for _, zdb := range meta {
		mappedIPs := mapIPs(zdb.IPs)
		ip, ok := mappedIPs[cfg.ZdbConnectionType]
		if !ok {
			return fmt.Errorf("ZDB connection type '%s' not supported on node for zdb %s", cfg.ZdbConnectionType, zdb.Name)
		}
		configBuilder.WriteString("\n\n[[meta.config.backends]]\n")
		configBuilder.WriteString(fmt.Sprintf("address = \"[%s]:9900\"\n", ip))
		configBuilder.WriteString(fmt.Sprintf("namespace = \"%s\"\n", zdb.Namespace))
		configBuilder.WriteString(fmt.Sprintf("password = \"%s\"", cfg.Password))
	}

	// Add data backends
	configBuilder.WriteString("\n\n[[groups]]")
	for _, zdb := range data {
		mappedIPs := mapIPs(zdb.IPs)
		ip, ok := mappedIPs[cfg.ZdbConnectionType]
		if !ok {
			return fmt.Errorf("ZDB connection type '%s' not supported on node for zdb %s", cfg.ZdbConnectionType, zdb.Name)
		}
		configBuilder.WriteString("\n\n[[groups.backends]]\n")
		configBuilder.WriteString(fmt.Sprintf("address = \"[%s]:9900\"\n", ip))
		configBuilder.WriteString(fmt.Sprintf("namespace = \"%s\"\n", zdb.Namespace))
		configBuilder.WriteString(fmt.Sprintf("password = \"%s\"", cfg.Password))
	}
	configBuilder.WriteString("\n")

	// Write config file
	if err := os.WriteFile(ConfigOutPath, []byte(configBuilder.String()), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Printf("Generated zstor config at %s\n", ConfigOutPath)
	return nil
}

func keyFromMnemonic(mnemonic, password string) (string, error) {
	seed := bip39.NewSeed(mnemonic, password)
	hash := sha256.Sum256(seed)
	return fmt.Sprintf("%x", hash), nil
}
