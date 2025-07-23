package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/grid"
	"github.com/threefoldtech/tfgrid-sdk-go/grid-client/workloads"
	bip39 "github.com/tyler-smith/go-bip39"
)

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
	Long: `Deploys metadata and data ZDBs on specified nodes.
Metadata ZDBs will be deployed with mode 'user' while data ZDBs will be 'seq'.`,
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
		if len(cfg.MetaNodes) == 0 {
			fmt.Println("Error: metadata nodes are required in config")
			os.Exit(1)
		}
		if len(cfg.DataNodes) == 0 {
			fmt.Println("Error: data nodes are required in config")
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

	// Create grid client
	network := Network
	if cfg.Network != "" {
		network = cfg.Network
	}
	gridClient, err := grid.NewGridClient(cfg.Mnemonic, network)
	if err != nil {
		return errors.Wrap(err, "failed to create grid client")
	}

	twinID := uint64(gridClient.TwinID)

	// Load existing deployments into state
	contracts, err := grid.GetContracts(&gridClient, twinID)
	if err != nil {
		return errors.Wrapf(err, "failed to query for existing contracts for twin %d", twinID)
	}

	if len(contracts) > 0 {
		fmt.Println("Found existing deployments, loading them into state...")
		for _, contractInfo := range contracts {
			name := contractInfo.DeploymentName
			expectedPrefix := fmt.Sprintf("%s_%d", cfg.DeploymentName, twinID)
			if !strings.HasPrefix(name, expectedPrefix) {
				continue
			}

			parts := strings.Split(name, "_")
			if len(parts) != 4 {
				continue
			}

			nodeID, err := strconv.ParseUint(parts[3], 10, 32)
			if err != nil {
				fmt.Printf("warn: could not parse nodeID from deployment name '%s': %v\n", name, err)
				continue
			}

			// First, store the contract ID in the state
			gridClient.State.StoreContractIDs(uint32(nodeID), uint64(contractInfo.Contract.ContractID))

			// This loads the deployment into grid.State
			if _, err := gridClient.State.LoadDeploymentFromGrid(context.Background(), uint32(nodeID), name); err != nil {
				fmt.Printf("warn: could not load deployment '%s' from grid: %v\n", name, err)
				continue
			}
		}
	}

	existingMetaNodes := make(map[uint32]bool)
	existingDataNodes := make(map[uint32]bool)
	for nodeID, contractIDs := range gridClient.State.CurrentNodeDeployments {
		for _, contractID := range contractIDs {
			contract, err := gridClient.SubstrateConn.GetContract(contractID)
			if err != nil {
				continue
			}
			var deploymentData struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal([]byte(contract.ContractType.NodeContract.DeploymentData), &deploymentData); err != nil {
				continue
			}
			name := deploymentData.Name
			expectedPrefix := fmt.Sprintf("%s_%d", cfg.DeploymentName, twinID)
			if !strings.HasPrefix(name, expectedPrefix) {
				continue
			}
			parts := strings.Split(name, "_")
			if len(parts) != 4 {
				continue
			}
			nodeType := parts[2]
			if nodeType == "meta" {
				existingMetaNodes[nodeID] = true
			} else if nodeType == "data" {
				existingDataNodes[nodeID] = true
			}
		}
	}
	if len(existingMetaNodes) > 0 || len(existingDataNodes) > 0 {
		fmt.Println("Finished loading, will only deploy missing ZDBs.")
	}

	// Prepare deployments
	var deploymentConfigs []*workloads.Deployment
	for _, nodeID := range cfg.MetaNodes {
		if existingMetaNodes[nodeID] {
			fmt.Printf("Skipping metadata deployment on node %d, already exists.\n", nodeID)
			continue
		}
		name := fmt.Sprintf("%s_%d_meta_%d", cfg.DeploymentName, twinID, nodeID)
		zdb := workloads.ZDB{
			Name:        name,
			Password:    cfg.Password,
			Public:      false,
			SizeGB:      uint64(cfg.MetaSizeGb),
			Description: "QSFS metadata namespace",
			Mode:        workloads.ZDBModeUser,
		}
		dl := workloads.NewDeployment(name, nodeID, "", nil, "", nil, []workloads.ZDB{zdb}, nil, nil, nil, nil)
		deploymentConfigs = append(deploymentConfigs, &dl)
	}

	for _, nodeID := range cfg.DataNodes {
		if existingDataNodes[nodeID] {
			fmt.Printf("Skipping data deployment on node %d, already exists.\n", nodeID)
			continue
		}
		name := fmt.Sprintf("%s_%d_data_%d", cfg.DeploymentName, twinID, nodeID)
		zdb := workloads.ZDB{
			Name:        name,
			Password:    cfg.Password,
			Public:      false,
			SizeGB:      uint64(cfg.DataSizeGb),
			Description: "QSFS data namespace",
			Mode:        workloads.ZDBModeSeq,
		}
		dl := workloads.NewDeployment(name, nodeID, "", nil, "", nil, []workloads.ZDB{zdb}, nil, nil, nil, nil)
		deploymentConfigs = append(deploymentConfigs, &dl)
	}

	// Batch deploy all ZDBs
	if len(deploymentConfigs) > 0 {
		fmt.Printf("Batch deploying %d ZDBs...\n", len(deploymentConfigs))
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if err := gridClient.DeploymentDeployer.BatchDeploy(ctx, deploymentConfigs); err != nil {
			return errors.Wrap(err, "failed to batch deploy ZDBs")
		}
	}

	// Load all deployed ZDBs (existing and new)
	metaDeployments := make([]*workloads.ZDB, len(cfg.MetaNodes))
	for i, nodeID := range cfg.MetaNodes {
		name := fmt.Sprintf("%s_%d_meta_%d", cfg.DeploymentName, twinID, nodeID)
		resZDB, err := gridClient.State.LoadZdbFromGrid(context.TODO(), nodeID, name, name)
		if err != nil {
			return errors.Wrapf(err, "failed to load deployed metadata ZDB '%s' from node %d", name, nodeID)
		}
		metaDeployments[i] = &resZDB
		fmt.Printf("Loaded metadata ZDB '%s' on node %d\n", name, nodeID)
	}

	dataDeployments := make([]*workloads.ZDB, len(cfg.DataNodes))
	for i, nodeID := range cfg.DataNodes {
		name := fmt.Sprintf("%s_%d_data_%d", cfg.DeploymentName, twinID, nodeID)
		resZDB, err := gridClient.State.LoadZdbFromGrid(context.TODO(), nodeID, name, name)
		if err != nil {
			return errors.Wrapf(err, "failed to load deployed data ZDB '%s' from node %d", name, nodeID)
		}
		dataDeployments[i] = &resZDB
		fmt.Printf("Loaded data ZDB '%s' on node %d\n", name, nodeID)
	}

	// Generate config file with all deployed ZDBs
	if err := generateRemoteConfig(cfg, metaDeployments, dataDeployments); err != nil {
		return errors.Wrap(err, "failed to generate config")
	}

	return nil
}

func generateRemoteConfig(cfg *Config, meta, data []*workloads.ZDB) error {
	key, err := keyFromMnemonic(cfg.Mnemonic, cfg.Password)
	if err != nil {
		return errors.Wrap(err, "failed to generate key from mnemonic")
	}

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
max_zdb_data_dir_size = 2560

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
key = "%s"`, cfg.MinShards, cfg.ExpectedShards, cfg.QsfsMountpoint, cfg.ZdbRootPath, key, key))

	// Add meta backends
	for _, zdb := range meta {
		configBuilder.WriteString("\n\n[[meta.config.backends]]\n")
		configBuilder.WriteString(fmt.Sprintf("address = \"[%s]:9900\"\n", zdb.IPs[len(zdb.IPs)-1]))
		configBuilder.WriteString(fmt.Sprintf("namespace = \"%s\"\n", zdb.Namespace))
		configBuilder.WriteString(fmt.Sprintf("password = \"%s\"", cfg.Password))
	}

	// Add data backends
	configBuilder.WriteString("\n\n[[groups]]")
	for _, zdb := range data {
		configBuilder.WriteString("\n\n[[groups.backends]]\n")
		configBuilder.WriteString(fmt.Sprintf("address = \"[%s]:9900\"\n", zdb.IPs[len(zdb.IPs)-1]))
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