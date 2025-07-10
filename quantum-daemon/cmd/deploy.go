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
	"gopkg.in/yaml.v3"
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
		if destroyDeployments {
			if err := destroyBackends(); err != nil {
				fmt.Printf("Error destroying deployments: %v\n", err)
				os.Exit(1)
			}
			return
		}
		if err := loadConfig(); err != nil {
			fmt.Printf("Error loading config: %v\n", err)
			os.Exit(1)
		}

		if len(AppConfig.MetaNodes) == 0 {
			fmt.Println("Error: metadata nodes are required in config")
			os.Exit(1)
		}
		if len(AppConfig.DataNodes) == 0 {
			fmt.Println("Error: data nodes are required in config")
			os.Exit(1)
		}
		if AppConfig.ZDBPass == "" {
			fmt.Println("Error: ZDB password is required in config")
			os.Exit(1)
		}
		if strings.ContainsAny(AppConfig.ZDBPass, "- ") {
			fmt.Println("Error: ZDB password cannot contain dashes or spaces")
			os.Exit(1)
		}

		if err := deployBackends(AppConfig.MetaNodes, AppConfig.DataNodes); err != nil {
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

func destroyBackends() error {
	if err := loadConfig(); err != nil {
		return err
	}

	relay := "wss://relay.grid.tf"
	network := Network
	if AppConfig.Network != "" {
		network = AppConfig.Network
	}
	if network != "main" {
		relay = fmt.Sprintf("wss://relay.%s.grid.tf", network)
	}

	grid, err := deployer.NewTFPluginClient(AppConfig.Mnemonic, 
		deployer.WithRelayURL(relay), 
		deployer.WithNetwork(network),
		deployer.WithRMBTimeout(100))
	if err != nil {
		return errors.Wrap(err, "failed to create grid client")
	}

	// Destroy metadata deployments
	for _, nodeID := range AppConfig.MetaNodes {
		projectName := fmt.Sprintf("vm/meta_%d", nodeID)
		if err := grid.CancelByProjectName(context.TODO(), projectName); err != nil {
			return errors.Wrapf(err, "failed to destroy metadata deployment on node %d", nodeID)
		}
		fmt.Printf("Destroyed metadata deployment on node %d\n", nodeID)
	}

	// Destroy data deployments
	for _, nodeID := range AppConfig.DataNodes {
		projectName := fmt.Sprintf("vm/data_%d", nodeID)
		if err := grid.CancelByProjectName(context.TODO(), projectName); err != nil {
			return errors.Wrapf(err, "failed to destroy data deployment on node %d", nodeID)
		}
		fmt.Printf("Destroyed data deployment on node %d\n", nodeID)
	}

	return nil
}

func loadConfig() error {
	if ConfigFile != "" {
		data, err := os.ReadFile(ConfigFile)
		if err != nil {
			return fmt.Errorf("failed to read config file: %w", err)
		}
		if err := yaml.Unmarshal(data, &AppConfig); err != nil {
			return fmt.Errorf("failed to parse config file: %w", err)
		}
	}

	// Override with ENV vars if set
	if env := os.Getenv("NETWORK"); env != "" {
		AppConfig.Network = env
	}
	if env := os.Getenv("MNEMONIC"); env != "" {
		AppConfig.Mnemonic = env
	}

	if AppConfig.Mnemonic == "" {
		return fmt.Errorf("mnemonic is required for deployment (provide via MNEMONIC env var or config file)")
	}

	return nil
}

func deployBackends(metaNodeIDs []uint32, dataNodeIDs []uint32) error {
	if err := loadConfig(); err != nil {
		return err
	}

	if len(AppConfig.MetaNodes) == 0 {
		return fmt.Errorf("metadata nodes are required in config")
	}
	if len(AppConfig.DataNodes) == 0 {
		return fmt.Errorf("data nodes are required in config")
	}
	if AppConfig.ZDBPass == "" {
		return fmt.Errorf("ZDB password is required in config")
	}
	// Create grid client
	relay := "wss://relay.grid.tf"
	network := Network
	if AppConfig.Network != "" {
		network = AppConfig.Network
	}
	if network != "main" {
		relay = fmt.Sprintf("wss://relay.%s.grid.tf", network)
	}
	grid, err := deployer.NewTFPluginClient(AppConfig.Mnemonic, deployer.WithRelayURL(relay), deployer.WithNetwork(network))
	if err != nil {
		return errors.Wrap(err, "failed to create grid client")
	}

	// Deploy metadata ZDBs
	metaDeployments := make([]*workloads.ZDB, len(metaNodeIDs))
	for i, nodeID := range metaNodeIDs {
		ns := fmt.Sprintf("meta_%d", nodeID)
		zdb := workloads.ZDB{
			Name:        ns,
			Password:    AppConfig.ZDBPass,
			Public:      false,
			SizeGB:      uint64(AppConfig.MetaSizeGB),
			Description: "QSFS metadata namespace",
			Mode:        workloads.ZDBModeUser,
		}

		dl := workloads.NewDeployment(fmt.Sprintf("meta_%d", nodeID), nodeID, "", nil, "", nil, []workloads.ZDB{zdb}, nil, nil, nil, nil)
		if err := grid.DeploymentDeployer.Deploy(context.TODO(), &dl); err != nil {
			return errors.Wrapf(err, "failed to deploy metadata ZDB on node %d", nodeID)
		}

		metaDeployments[i] = &zdb
		fmt.Printf("Deployed metadata ZDB '%s' on node %d\n", ns, nodeID)
	}

	// Deploy data ZDBs
	dataDeployments := make([]*workloads.ZDB, len(dataNodeIDs))
	for i, nodeID := range dataNodeIDs {
		ns := fmt.Sprintf("data_%d", nodeID)
		zdb := workloads.ZDB{
			Name:        ns,
			Password:    AppConfig.ZDBPass,
			Public:      false,
			SizeGB:      uint64(AppConfig.DataSizeGB),
			Description: "QSFS data namespace",
			Mode:        workloads.ZDBModeSeq,
		}

		dl := workloads.NewDeployment(fmt.Sprintf("data_%d", nodeID), nodeID, "", nil, "", nil, []workloads.ZDB{zdb}, nil, nil, nil, nil)
		if err := grid.DeploymentDeployer.Deploy(context.TODO(), &dl); err != nil {
			return errors.Wrapf(err, "failed to deploy data ZDB on node %d", nodeID)
		}

		dataDeployments[i] = &zdb
		fmt.Printf("Deployed data ZDB '%s' on node %d\n", ns, nodeID)
	}

	// Generate config file with all deployed ZDBs
	if err := generateRemoteConfig(metaDeployments, dataDeployments); err != nil {
		return errors.Wrap(err, "failed to generate config")
	}

	return nil
}

func generateRemoteConfig(meta, data []*workloads.ZDB) error {
	config := fmt.Sprintf(`minimal_shards = 2
expected_shards = 4
redundant_groups = 0
redundant_nodes = 0
root = "/"
zdbfs_mountpoint = "/mnt/qsfs"
socket = "/tmp/zstor.sock"
prometheus_port = 9200
zdb_data_dir_path = "/data/data/zdbfs-data/"
max_zdb_data_dir_size = 2560

[compression]
algorithm = "snappy"

[meta]
type = "zdb"

[meta.config]
prefix = "zstor-meta"

[encryption]
algorithm = "AES"
key = "%x"

[meta.config.encryption]
algorithm = "AES"
key = "%x"`, randomKey(), randomKey())

	// Add meta backends
	config += "\n\n[[meta.config.backends]]\n"
	for _, zdb := range meta {
		config += fmt.Sprintf("address = \"[%s]:9900\"\n", zdb.IPs[0])
		config += fmt.Sprintf("namespace = \"%s\"\n", zdb.Name)
		config += fmt.Sprintf("password = \"%s\"\n\n", AppConfig.ZDBPass)
		if zdb != meta[len(meta)-1] {
			config += "[[meta.config.backends]]\n"
		}
	}

	// Add data backends
	config += "[[groups]]\n"
	for _, zdb := range data {
		config += fmt.Sprintf("[[groups.backends]]\n")
		config += fmt.Sprintf("address = \"[%s]:9900\"\n", zdb.IPs[0])
		config += fmt.Sprintf("namespace = \"%s\"\n", zdb.Name)
		config += fmt.Sprintf("password = \"%s\"\n\n", AppConfig.ZDBPass)
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
	return fmt.Sprintf("%x", key)
}
