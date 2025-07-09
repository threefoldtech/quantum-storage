package cmd

import (
	"context"
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

var (
	metaNodes    string
	dataNodes    string
	zdbPassword  string
	metaSizeGB   int
	dataSizeGB   int
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
		if Mnemonic == "" {
			fmt.Println("Error: mnemonic is required for deployment (provide via --mnemonic or MNEMONIC env var)")
			os.Exit(1)
		}

		metaNodeIDs, err := parseNodeIDs(metaNodes)
		if err != nil {
			fmt.Printf("Error parsing metadata nodes: %v\n", err)
			os.Exit(1)
		}
		dataNodeIDs, err := parseNodeIDs(dataNodes)
		if err != nil {
			fmt.Printf("Error parsing data nodes: %v\n", err)
			os.Exit(1)
		}

		if len(metaNodeIDs) == 0 || len(dataNodeIDs) == 0 {
			fmt.Println("Error: both metadata and data nodes must be specified")
			os.Exit(1)
		}

		if strings.ContainsAny(zdbPassword, "- ") {
			fmt.Println("Error: ZDB password cannot contain dashes or spaces")
			os.Exit(1)
		}

		if err := deployBackends(metaNodeIDs, dataNodeIDs); err != nil {
			fmt.Printf("Error deploying backends: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	deployCmd.Flags().StringVarP(&metaNodes, "meta-nodes", "", "", "Comma-separated list of node IDs for metadata ZDBs (overrides config)")
	deployCmd.Flags().StringVarP(&dataNodes, "data-nodes", "", "", "Comma-separated list of node IDs for data ZDBs (overrides config)")
	deployCmd.Flags().StringVarP(&zdbPassword, "password", "p", "", "Password to use for ZDB namespaces (overrides config)")
	deployCmd.Flags().IntVarP(&metaSizeGB, "meta-size", "", 1, "Size in GB for metadata ZDBs (overrides config)")
	deployCmd.Flags().IntVarP(&dataSizeGB, "data-size", "", 10, "Size in GB for data ZDBs (overrides config)")
	rootCmd.AddCommand(deployCmd)
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

	return nil
}

func deployBackends(metaNodeIDs []uint32, dataNodeIDs []uint32) error {
	if err := loadConfig(); err != nil {
		return err
	}

	// Override config with CLI flags if provided
	if metaNodes != "" {
		ids, err := parseNodeIDs(metaNodes)
		if err != nil {
			return fmt.Errorf("invalid meta nodes: %w", err)
		}
		metaNodeIDs = ids
	} else if len(AppConfig.MetaNodes) > 0 {
		metaNodeIDs = AppConfig.MetaNodes
	}

	if dataNodes != "" {
		ids, err := parseNodeIDs(dataNodes)
		if err != nil {
			return fmt.Errorf("invalid data nodes: %w", err)
		}
		dataNodeIDs = ids
	} else if len(AppConfig.DataNodes) > 0 {
		dataNodeIDs = AppConfig.DataNodes
	}

	if zdbPassword != "" {
		AppConfig.ZDBPass = zdbPassword
	} else if AppConfig.ZDBPass == "" {
		return fmt.Errorf("ZDB password is required (provide via --password or config)")
	}

	if metaSizeGB != 1 {
		AppConfig.MetaSizeGB = metaSizeGB
	}

	if dataSizeGB != 10 {
		AppConfig.DataSizeGB = dataSizeGB
	}
	// Create grid client
	relay := "wss://relay.grid.tf"
	if Network != "main" && Network != "" {
		relay = fmt.Sprintf("wss://relay.%s.grid.tf", Network)
	}
	grid, err := deployer.NewTFPluginClient(Mnemonic, deployer.WithRelayURL(relay), deployer.WithNetwork(Network))
	if err != nil {
		return errors.Wrap(err, "failed to create grid client")
	}

	// Deploy metadata ZDBs
	metaDeployments := make([]*workloads.ZDB, len(metaNodes))
	for i, nodeID := range metaNodeIDs {
		ns := fmt.Sprintf("meta_%d", nodeID)
		zdb := workloads.ZDB{
			Name:        ns,
			Password:    zdbPassword,
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
	dataDeployments := make([]*workloads.ZDB, len(dataNodes))
	for i, nodeID := range dataNodeIDs {
		ns := fmt.Sprintf("data_%d", nodeID)
		zdb := workloads.ZDB{
			Name:        ns,
			Password:    zdbPassword,
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
	// TODO: Implement config generation similar to generateLocalConfig()
	// but using the deployed ZDB information
	return nil
}
