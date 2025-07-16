package cmd

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/threefoldtech/tfgrid-sdk-go/grid-client/deployer"
	"github.com/threefoldtech/tfgrid-sdk-go/grid-client/workloads"
	"github.com/threefoldtech/tfgrid-sdk-go/grid-proxy/pkg/types"
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
Metadata ZDBs will be deployed with mode 'user' while data ZDBs will be 'seq'.
Use --destroy to remove existing deployments.`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := LoadConfig(ConfigFile)
		if err != nil {
			fmt.Printf("Error loading config: %v\n", err)
			os.Exit(1)
		}

		if destroyDeployments {
			if err := destroyBackends(cfg); err != nil {
				fmt.Printf("Error destroying deployments: %v\n", err)
				os.Exit(1)
			}
			return
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

		if err := deployBackends(cfg); err != nil {
			fmt.Printf("Error deploying backends: %v\n", err)
			os.Exit(1)
		}
	},
}

var destroyDeployments bool

func init() {
	deployCmd.Flags().BoolVarP(&destroyDeployments, "destroy", "d", false, "Destroy existing deployments")
	deployCmd.Flags().StringVarP(&ConfigOutPath, "out", "o", "/etc/zstor.toml", "Path to write generated zstor config")
	rootCmd.AddCommand(deployCmd)
}

func destroyBackends(cfg *Config) error {
	if cfg.DeploymentName == "" {
		return errors.New("deployment_name is required in config for destroying")
	}

	relay := "wss://relay.grid.tf"
	network := Network
	if cfg.Network != "" {
		network = cfg.Network
	}
	if network != "main" {
		relay = fmt.Sprintf("wss://relay.%s.grid.tf", network)
	}

	grid, err := deployer.NewTFPluginClient(cfg.Mnemonic,
		deployer.WithRelayURL(relay),
		deployer.WithNetwork(network),
		deployer.WithRMBTimeout(100))
	if err != nil {
		return errors.Wrap(err, "failed to create grid client")
	}

	ctx := context.Background()
	twinID := grid.TwinID
	twinID64 := uint64(twinID)
	filter := types.ContractFilter{
		TwinID: &twinID64,
		State:  []string{"Created"},
	}
	limit := types.Limit{
		Size: 100, // Should be enough for most deployments
	}

	contracts, _, err := grid.GridProxyClient.Contracts(ctx, filter, limit)
	if err != nil {
		return errors.Wrapf(err, "failed to query contracts for twin %d", twinID)
	}

	var contractsToCancel []types.Contract
	for _, contract := range contracts {
		var name string
		if contract.Type == "name" {
			if details, ok := contract.Details.(map[string]interface{}); ok {
				if n, ok := details["name"].(string); ok {
					name = n
				}
			}
		} else if contract.Type == "node" {
			if details, ok := contract.Details.(types.NodeContractDetails); ok {
				if deploymentData := details.DeploymentData; deploymentData != "" {
					var data struct {
						Name string `json:"name"`
					}
					if err := json.Unmarshal([]byte(deploymentData), &data); err == nil {
						name = data.Name
					}
				}
			}
		}

		if strings.HasPrefix(name, cfg.DeploymentName) {
			parts := strings.Split(name, "_")
			if len(parts) == 4 && parts[0] == cfg.DeploymentName && (parts[2] == "meta" || parts[2] == "data") {
				contractsToCancel = append(contractsToCancel, contract)
			}
		}
	}

	if len(contractsToCancel) == 0 {
		fmt.Printf("No deployments found with name starting with '%s'. Nothing to do.\n", cfg.DeploymentName)
		return nil
	}

	fmt.Printf("Found %d deployments to destroy.\n", len(contractsToCancel))

	for _, contract := range contractsToCancel {
		fmt.Printf("Destroying deployment with contract ID %d\n", contract.ContractID)
		if err := grid.SubstrateConn.CancelContract(grid.Identity, uint64(contract.ContractID)); err != nil {
			fmt.Printf("warn: failed to destroy deployment with contract ID %d: %v\n", contract.ContractID, err)
		} else {
			fmt.Printf("Destroyed deployment with contract ID %d\n", contract.ContractID)
		}
	}

	return nil
}

func newRandom(n int) (string, error) {
	bytes := make([]byte, n)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func deployBackends(cfg *Config) error {
	// Create grid client
	relay := "wss://relay.grid.tf"
	network := Network
	if cfg.Network != "" {
		network = cfg.Network
	}
	if network != "main" {
		relay = fmt.Sprintf("wss://relay.%s.grid.tf", network)
	}
	grid, err := deployer.NewTFPluginClient(cfg.Mnemonic, deployer.WithRelayURL(relay), deployer.WithNetwork(network))
	if err != nil {
		return errors.Wrap(err, "failed to create grid client")
	}

	ctx := context.Background()
	twinID := grid.TwinID
	twinID64 := uint64(twinID)
	filter := types.ContractFilter{
		TwinID: &twinID64,
	}
	limit := types.Limit{
		Size: 200, // Should be enough
	}
	contracts, _, err := grid.GridProxyClient.Contracts(ctx, filter, limit)
	if err != nil {
		return errors.Wrapf(err, "failed to query for existing contracts for twin %d", twinID)
	}

	var randoms string
	existingMetaNodes := make(map[uint32]bool)
	existingDataNodes := make(map[uint32]bool)

	if len(contracts) > 0 {
		fmt.Println("Found existing deployments, will only deploy missing ZDBs.")
		// Extract random string and existing nodes
		for _, contract := range contracts {
			var name string
			if contract.Type == "name" {
				if details, ok := contract.Details.(map[string]interface{}); ok {
					if n, ok := details["name"].(string); ok {
						name = n
					}
				}
			} else if contract.Type == "node" {
				if details, ok := contract.Details.(map[string]interface{}); ok {
					if deploymentData, ok := details["deployment_data"].(string); ok {
						var data struct {
							Name string `json:"name"`
						}
						if err := json.Unmarshal([]byte(deploymentData), &data); err == nil {
							name = data.Name
						}
					}
				}
			}

			if !strings.HasPrefix(name, cfg.DeploymentName) {
				continue
			}

			parts := strings.Split(name, "_")
			if len(parts) != 4 || parts[0] != cfg.DeploymentName {
				continue
			}
			if randoms == "" {
				randoms = parts[1]
				fmt.Printf("Using existing random identifier: %s\n", randoms)
			}
			nodeType := parts[2]
			nodeID, err := strconv.ParseUint(parts[3], 10, 32)
			if err != nil {
				continue
			}
			if nodeType == "meta" {
				existingMetaNodes[uint32(nodeID)] = true
			} else if nodeType == "data" {
				existingDataNodes[uint32(nodeID)] = true
			}
		}
	}

	if randoms == "" {
		randoms, err = newRandom(4)
		if err != nil {
			return errors.Wrap(err, "failed to generate random string")
		}
		fmt.Printf("Generated new random identifier: %s\n", randoms)
	}

	// Prepare metadata deployments
	var metaDeploymentConfigs []*workloads.Deployment
	for _, nodeID := range cfg.MetaNodes {
		if existingMetaNodes[nodeID] {
			fmt.Printf("Skipping metadata deployment on node %d, already exists.\n", nodeID)
			continue
		}
		name := fmt.Sprintf("%s_%s_meta_%d", cfg.DeploymentName, randoms, nodeID)
		zdb := workloads.ZDB{
			Name:        name,
			Password:    cfg.Password,
			Public:      false,
			SizeGB:      uint64(cfg.MetaSizeGb),
			Description: "QSFS metadata namespace",
			Mode:        workloads.ZDBModeUser,
		}
		dl := workloads.NewDeployment(name, nodeID, "", nil, "", nil, []workloads.ZDB{zdb}, nil, nil, nil, nil)
		metaDeploymentConfigs = append(metaDeploymentConfigs, &dl)
	}

	// Prepare data deployments
	var dataDeploymentConfigs []*workloads.Deployment
	for _, nodeID := range cfg.DataNodes {
		if existingDataNodes[nodeID] {
			fmt.Printf("Skipping data deployment on node %d, already exists.\n", nodeID)
			continue
		}
		name := fmt.Sprintf("%s_%s_data_%d", cfg.DeploymentName, randoms, nodeID)
		zdb := workloads.ZDB{
			Name:        name,
			Password:    cfg.Password,
			Public:      false,
			SizeGB:      uint64(cfg.DataSizeGb),
			Description: "QSFS data namespace",
			Mode:        workloads.ZDBModeSeq,
		}
		dl := workloads.NewDeployment(name, nodeID, "", nil, "", nil, []workloads.ZDB{zdb}, nil, nil, nil, nil)
		dataDeploymentConfigs = append(dataDeploymentConfigs, &dl)
	}

	// Batch deploy metadata ZDBs
	if len(metaDeploymentConfigs) > 0 {
		fmt.Printf("Batch deploying %d metadata ZDBs...\n", len(metaDeploymentConfigs))
		if err := grid.DeploymentDeployer.BatchDeploy(context.TODO(), metaDeploymentConfigs); err != nil {
			return errors.Wrap(err, "failed to batch deploy metadata ZDBs")
		}
	}

	// Batch deploy data ZDBs
	if len(dataDeploymentConfigs) > 0 {
		fmt.Printf("Batch deploying %d data ZDBs...\n", len(dataDeploymentConfigs))
		if err := grid.DeploymentDeployer.BatchDeploy(context.TODO(), dataDeploymentConfigs); err != nil {
			return errors.Wrap(err, "failed to batch deploy data ZDBs")
		}
	}

	// Load all deployed ZDBs (existing and new)
	metaDeployments := make([]*workloads.ZDB, len(cfg.MetaNodes))
	for i, nodeID := range cfg.MetaNodes {
		name := fmt.Sprintf("%s_%s_meta_%d", cfg.DeploymentName, randoms, nodeID)
		resZDB, err := grid.State.LoadZdbFromGrid(context.TODO(), nodeID, name, name)
		if err != nil {
			return errors.Wrapf(err, "failed to load deployed metadata ZDB '%s' from node %d", name, nodeID)
		}
		metaDeployments[i] = &resZDB
		fmt.Printf("Loaded metadata ZDB '%s' on node %d\n", name, nodeID)
	}

	dataDeployments := make([]*workloads.ZDB, len(cfg.DataNodes))
	for i, nodeID := range cfg.DataNodes {
		name := fmt.Sprintf("%s_%s_data_%d", cfg.DeploymentName, randoms, nodeID)
		resZDB, err := grid.State.LoadZdbFromGrid(context.TODO(), nodeID, name, name)
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
