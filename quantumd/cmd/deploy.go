package cmd

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/grid"
	"github.com/threefoldtech/tfgrid-sdk-go/grid-client/deployer"
	"github.com/threefoldtech/tfgrid-sdk-go/grid-client/workloads"
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

func parseNodeIDs(input string) ([]uint32, error) {
	parts := strings.Split(input, ",")
	ids := make([]uint32, 0, len(parts))
	for _, part := range parts {
		id, err := strconv.ParseUint(strings.TrimSpace(part), 10, 32)
		if err != nil {
			return nil, err
		}
		ids = append(ids, uint32(id))
	}
	return ids, nil
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

		if cfg.DeploymentName == "" {
			fmt.Println("Error: deployment_name is required in config")
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

func DeployBackends(cfg *Config) error {
	network := Network
	if cfg.Network != "" {
		network = cfg.Network
	}
	gridClient, err := grid.NewGridClient(cfg.Mnemonic, network)
	if err != nil {
		return errors.Wrap(err, "failed to create grid client")
	}

	twinID := uint64(gridClient.TwinID)

	// Load existing deployments
	existingDeployments, err := loadExistingDeployments(&gridClient, twinID, cfg.DeploymentName)
	if err != nil {
		return errors.Wrap(err, "failed to load existing deployments")
	}
	fmt.Printf("Found %d existing deployments.\n", len(existingDeployments))

	// Node pool for automatic selection
	nodePool := newNodePool(cfg, &gridClient, existingDeployments)

	// Deploy metadata ZDBs
	metaDeployments, err := deployZDBsWithRetry(
		&gridClient, cfg, "meta", cfg.MetaSizeGb, workloads.ZDBModeUser, 4,
		cfg.MetaNodes, existingDeployments, nodePool,
	)
	if err != nil {
		return errors.Wrap(err, "failed to deploy metadata ZDBs")
	}

	// Deploy data ZDBs
	dataDeployments, err := deployZDBsWithRetry(
		&gridClient, cfg, "data", cfg.DataSizeGb, workloads.ZDBModeSeq, cfg.ExpectedShards,
		cfg.DataNodes, existingDeployments, nodePool,
	)
	if err != nil {
		return errors.Wrap(err, "failed to deploy data ZDBs")
	}

	// Generate config
	return generateRemoteConfig(cfg, metaDeployments, dataDeployments)
}

// Helper to load existing deployments from the grid
func loadExistingDeployments(gridClient *deployer.TFPluginClient, twinID uint64, deploymentName string) (map[uint32]string, error) {
	existing := make(map[uint32]string)
	contracts, err := grid.GetContracts(gridClient, twinID)
	if err != nil {
		return nil, err
	}

	for _, contractInfo := range contracts {
		name := contractInfo.DeploymentName
		expectedPrefix := fmt.Sprintf("%s_%d", deploymentName, twinID)
		if !strings.HasPrefix(name, expectedPrefix) {
			continue
		}

		parts := strings.Split(name, "_")
		if len(parts) != 4 {
			continue
		}
		nodeID, err := strconv.ParseUint(parts[3], 10, 32)
		if err != nil {
			continue
		}

		// This loads the deployment into grid.State, which is important for loading ZDB details later
		if _, err := gridClient.State.LoadDeploymentFromGrid(context.Background(), uint32(nodeID), name); err != nil {
			fmt.Printf("warn: could not load deployment '%s' from grid: %v\n", name, err)
			continue
		}
		existing[uint32(nodeID)] = parts[2] // Store nodeID -> type ("meta" or "data")
	}
	return existing, nil
}

// newNodePool creates a helper for managing available nodes for deployment.
func newNodePool(cfg *Config, gridClient *deployer.TFPluginClient, existing map[uint32]string) *nodePool {
	return &nodePool{
		cfg:        cfg,
		gridClient: gridClient,
		used:       existing,
		mu:         sync.Mutex{},
	}
}

type nodePool struct {
	cfg        *Config
	gridClient *deployer.TFPluginClient
	used       map[uint32]string // nodeID -> type
	mu         sync.Mutex
}

func (p *nodePool) Get(count int) ([]uint32, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.cfg.Farms) == 0 {
		return nil, errors.New("no farms configured for automatic node selection")
	}

	// Collect all nodes that have been used in any capacity
	excludedNodes := make(map[uint32]bool)
	for nodeID := range p.used {
		excludedNodes[nodeID] = true
	}

	// Fetch available nodes from farms
	hru := uint64(p.cfg.DataSizeGb) * 1024 * 1024 * 1024 // Use data size for filtering
	availableNodes, err := grid.GetNodesFromFarms(p.gridClient, p.cfg.Farms, hru, 0)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get nodes from farms")
	}

	// Filter out excluded nodes
	candidates := []uint32{}
	for _, nodeID := range availableNodes {
		if !excludedNodes[nodeID] {
			candidates = append(candidates, nodeID)
		}
	}

	if len(candidates) < count {
		return nil, fmt.Errorf("not enough available nodes in farms, needed %d, found %d", count, len(candidates))
	}

	rand.Shuffle(len(candidates), func(i, j int) {
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})

	return candidates[:count], nil
}

func (p *nodePool) MarkUsed(nodeID uint32, nodeType string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.used[nodeID] = nodeType
}

// deployZDBsWithRetry handles the entire lifecycle of deploying a group of ZDBs,
// including retries on failure.
func deployZDBsWithRetry(
	gridClient *deployer.TFPluginClient,
	cfg *Config,
	nodeType string,
	sizeGB int,
	mode string,
	requiredCount int,
	manualNodes []uint32,
	existing map[uint32]string,
	pool *nodePool,
) ([]*workloads.ZDB, error) {


deployments := []*workloads.ZDB{}
	nodesToDeploy := []uint32{}

	// First, check existing and manual nodes
	for _, nodeID := range manualNodes {
		if t, ok := existing[nodeID]; ok && t == nodeType {
			fmt.Printf("Found existing %s deployment on manually specified node %d.\n", nodeType, nodeID)
			if zdb, err := loadZDB(gridClient, cfg, nodeID, nodeType); err == nil {
				deployments = append(deployments, zdb)
				pool.MarkUsed(nodeID, nodeType)
			} else {
				fmt.Printf("warn: could not load existing zdb from node %d: %v\n", nodeID, err)
			}
		} else {
			nodesToDeploy = append(nodesToDeploy, nodeID)
		}
	}

	// Loop until we have enough deployments
	for len(deployments) < requiredCount {
		needed := requiredCount - len(deployments)

		// Get candidate nodes
		var candidates []uint32
		if len(nodesToDeploy) > 0 {
			candidates = nodesToDeploy
			nodesToDeploy = nil // Use them up
		} else {
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

		// Deploy in parallel
		type result struct {
			zdb    *workloads.ZDB
			nodeID uint32
			err    error
		}
		results := make(chan result, len(candidates))
		var wg sync.WaitGroup

		for _, nodeID := range candidates {
			wg.Add(1)
			go func(nodeID uint32) {
				defer wg.Done()
				pool.MarkUsed(nodeID, nodeType) // Mark as used immediately to avoid re-selection
				zdb, err := deployZDB(gridClient, cfg, nodeID, nodeType, sizeGB, mode)
				results <- result{zdb, nodeID, err}
			}(nodeID)
		}

		wg.Wait()
		close(results)

		for res := range results {
			if res.err != nil {
				fmt.Printf("warn: deployment failed: %v. Will try to find a replacement.\n", res.err)
			} else {
				deployments = append(deployments, res.zdb)
				fmt.Printf("Successfully deployed %s ZDB on node %d\n", nodeType, res.nodeID)
			}
		}

		if len(deployments) < requiredCount {
			fmt.Printf("Deployed %d/%d %s ZDBs, retrying...\n", len(deployments), requiredCount, nodeType)
			time.Sleep(2 * time.Second) // Brief pause before retry
		}
	}

	fmt.Printf("Successfully deployed all %d %s ZDBs.\n", requiredCount, nodeType)
	return deployments, nil
}

// deployZDB performs a single ZDB deployment.
func deployZDB(
	gridClient *deployer.TFPluginClient,
	cfg *Config,
	nodeID uint32,
	nodeType string,
	sizeGB int,
	mode string,
) (*workloads.ZDB, error) {

	twinID := uint64(gridClient.TwinID)
	name := fmt.Sprintf("%s_%d_%s_%d", cfg.DeploymentName, twinID, nodeType, nodeID)

	fmt.Printf("Deploying %s ZDB '%s' on node %d...\n", nodeType, name, nodeID)

	zdb := workloads.ZDB{
		Name:        name,
		Password:    cfg.Password,
		Public:      false,
		SizeGB:      uint64(sizeGB),
		Description: fmt.Sprintf("QSFS %s namespace", nodeType),
		Mode:        mode,
	}
	dl := workloads.NewDeployment(name, nodeID, "", nil, "", nil, []workloads.ZDB{zdb}, nil, nil, nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := gridClient.DeploymentDeployer.Deploy(ctx, &dl); err != nil {
		return nil, errors.Wrapf(err, "failed to deploy %s ZDB on node %d", nodeType, nodeID)
	}

	resZDB, err := gridClient.State.LoadZdbFromGrid(context.TODO(), nodeID, name, name)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load deployed ZDB '%s' from node %d", name, nodeID)
	}
	return &resZDB, nil
}

// loadZDB is a helper to load a ZDB from the grid.
func loadZDB(gridClient *deployer.TFPluginClient, cfg *Config, nodeID uint32, nodeType string) (*workloads.ZDB, error) {
	twinID := uint64(gridClient.TwinID)
	name := fmt.Sprintf("%s_%d_%s_%d", cfg.DeploymentName, twinID, nodeType, nodeID)
	resZDB, err := gridClient.State.LoadZdbFromGrid(context.TODO(), nodeID, name, name)
	if err != nil {
		return nil, err
	}
	return &resZDB, nil
}

func generateRemoteConfig(cfg *Config, meta, data []*workloads.ZDB) error {
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
