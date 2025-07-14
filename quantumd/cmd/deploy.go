package cmd

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/threefoldtech/tfgrid-sdk-go/grid-client/deployer"
	"github.com/threefoldtech/tfgrid-sdk-go/grid-client/workloads"
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

		if len(cfg.MetaNodes) == 0 {
			fmt.Println("Error: metadata nodes are required in config")
			os.Exit(1)
		}
		if len(cfg.DataNodes) == 0 {
			fmt.Println("Error: data nodes are required in config")
			os.Exit(1)
		}
		if cfg.ZdbPassword == "" {
			fmt.Println("Error: ZDB password is required in config")
			os.Exit(1)
		}
		if strings.ContainsAny(cfg.ZdbPassword, "- ") {
			fmt.Println("Error: ZDB password cannot contain dashes or spaces")
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
	deployCmd.Flags().StringVarP(&ConfigOutPath, "out", "o", "/etc/zstor-default.toml", "Path to write generated zstor config")
	rootCmd.AddCommand(deployCmd)
}

func destroyBackends(cfg *Config) error {
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

	// Destroy metadata deployments
	for _, nodeID := range cfg.MetaNodes {
		projectName := fmt.Sprintf("meta_%d", nodeID)
		if err := grid.CancelByProjectName(projectName); err != nil {
			return errors.Wrapf(err, "failed to destroy metadata deployment on node %d", nodeID)
		}
		fmt.Printf("Destroyed metadata deployment on node %d\n", nodeID)
	}

	// Destroy data deployments
	for _, nodeID := range cfg.DataNodes {
		projectName := fmt.Sprintf("data_%d", nodeID)
		if err := grid.CancelByProjectName(projectName); err != nil {
			return errors.Wrapf(err, "failed to destroy data deployment on node %d", nodeID)
		}
		fmt.Printf("Destroyed data deployment on node %d\n", nodeID)
	}

	return nil
}

func deployBackends(cfg *Config) error {
	if len(cfg.MetaNodes) == 0 {
		return fmt.Errorf("metadata nodes are required in config")
	}
	if len(cfg.DataNodes) == 0 {
		return fmt.Errorf("data nodes are required in config")
	}
	if cfg.ZdbPassword == "" {
		return fmt.Errorf("ZDB password is required in config")
	}
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

	// Prepare metadata deployments
	var metaDeploymentConfigs []*workloads.Deployment
	for _, nodeID := range cfg.MetaNodes {
		ns := fmt.Sprintf("meta_%d", nodeID)
		zdb := workloads.ZDB{
			Name:        ns,
			Password:    cfg.ZdbPassword,
			Public:      false,
			SizeGB:      uint64(cfg.MetaSizeGb),
			Description: "QSFS metadata namespace",
			Mode:        workloads.ZDBModeUser,
		}

		dl := workloads.NewDeployment(fmt.Sprintf("meta_%d", nodeID), nodeID, "", nil, "", nil, []workloads.ZDB{zdb}, nil, nil, nil, nil)
		metaDeploymentConfigs = append(metaDeploymentConfigs, &dl)
	}

	// Prepare data deployments
	var dataDeploymentConfigs []*workloads.Deployment
	for _, nodeID := range cfg.DataNodes {
		ns := fmt.Sprintf("data_%d", nodeID)
		zdb := workloads.ZDB{
			Name:        ns,
			Password:    cfg.ZdbPassword,
			Public:      false,
			SizeGB:      uint64(cfg.DataSizeGb),
			Description: "QSFS data namespace",
			Mode:        workloads.ZDBModeSeq,
		}

		dl := workloads.NewDeployment(fmt.Sprintf("data_%d", nodeID), nodeID, "", nil, "", nil, []workloads.ZDB{zdb}, nil, nil, nil, nil)
		dataDeploymentConfigs = append(dataDeploymentConfigs, &dl)
	}

	// Batch deploy metadata ZDBs
	fmt.Printf("Batch deploying %d metadata ZDBs...\n", len(metaDeploymentConfigs))
	if err := grid.DeploymentDeployer.BatchDeploy(context.TODO(), metaDeploymentConfigs); err != nil {
		return errors.Wrap(err, "failed to batch deploy metadata ZDBs")
	}

	// Batch deploy data ZDBs
	fmt.Printf("Batch deploying %d data ZDBs...\n", len(dataDeploymentConfigs))
	if err := grid.DeploymentDeployer.BatchDeploy(context.TODO(), dataDeploymentConfigs); err != nil {
		return errors.Wrap(err, "failed to batch deploy data ZDBs")
	}

	// Load deployed metadata ZDBs
	metaDeployments := make([]*workloads.ZDB, len(cfg.MetaNodes))
	for i, nodeID := range cfg.MetaNodes {
		ns := fmt.Sprintf("meta_%d", nodeID)
		dlName := fmt.Sprintf("meta_%d", nodeID)

		resZDB, err := grid.State.LoadZdbFromGrid(context.TODO(), nodeID, ns, dlName)
		if err != nil {
			return errors.Wrapf(err, "failed to load deployed metadata ZDB '%s' from node %d", ns, nodeID)
		}

		metaDeployments[i] = &resZDB
		fmt.Printf("Deployed metadata ZDB '%s' on node %d\n", ns, nodeID)
	}

	// Load deployed data ZDBs
	dataDeployments := make([]*workloads.ZDB, len(cfg.DataNodes))
	for i, nodeID := range cfg.DataNodes {
		ns := fmt.Sprintf("data_%d", nodeID)
		dlName := fmt.Sprintf("data_%d", nodeID)

		resZDB, err := grid.State.LoadZdbFromGrid(context.TODO(), nodeID, ns, dlName)
		if err != nil {
			return errors.Wrapf(err, "failed to load deployed data ZDB '%s' from node %d", ns, nodeID)
		}

		dataDeployments[i] = &resZDB
		fmt.Printf("Deployed data ZDB '%s' on node %d\n", ns, nodeID)
	}

	// Generate config file with all deployed ZDBs
	if err := generateRemoteConfig(cfg, metaDeployments, dataDeployments); err != nil {
		return errors.Wrap(err, "failed to generate config")
	}

	return nil
}

func generateRemoteConfig(cfg *Config, meta, data []*workloads.ZDB) error {
	config := fmt.Sprintf(`minimal_shards = %d
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
key = "%s"`, cfg.MinShards, cfg.ExpectedShards, cfg.QsfsMountpoint, cfg.ZdbRootPath, randomKey(), randomKey())

	// Add meta backends
	config += "\n\n[[meta.config.backends]]\n"
	for _, zdb := range meta {
		config += fmt.Sprintf("address = \"[%s]:9900\"\n", zdb.IPs[len(zdb.IPs)-1])
		config += fmt.Sprintf("namespace = \"%s\"\n", zdb.Namespace)
		config += fmt.Sprintf("password = \"%s\"\n\n", cfg.ZdbPassword)
		if zdb != meta[len(meta)-1] {
			config += "[[meta.config.backends]]\n"
		}
	}

	// Add data backends
	config += "[[groups]]\n"
	for _, zdb := range data {
		config += fmt.Sprintf("[[groups.backends]]\n")
		config += fmt.Sprintf("address = \"[%s]:9900\"\n", zdb.IPs[len(zdb.IPs)-1])
		config += fmt.Sprintf("namespace = \"%s\"\n", zdb.Namespace)
		config += fmt.Sprintf("password = \"%s\"\n\n", cfg.ZdbPassword)
	}

	// Write config file
	if err := os.WriteFile(ConfigOutPath, []byte(config), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Printf("Generated zstor config at %s\n", ConfigOutPath)
	return nil
}

func randomKey() string {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic("failed to generate random key")
	}
	// Format as 32 hex bytes (64 chars) without spaces or dashes
	return fmt.Sprintf("%064x", key)
}