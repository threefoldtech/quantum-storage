package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/threefoldtech/tfgrid-sdk-go/grid-client/deployer"
	"github.com/threefoldtech/tfgrid-sdk-go/grid-client/workloads"
)

var (
	metaNodes []uint32
	dataNodes []uint32
)

var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy backend ZDBs on the ThreeFold Grid",
	Long: `Deploys metadata and data ZDBs on specified nodes.
Metadata ZDBs will be deployed with mode 'user' while data ZDBs will be 'seq'.`,
	Run: func(cmd *cobra.Command, args []string) {
		if Mnemonic == "" {
			fmt.Println("Error: mnemonic is required for deployment")
			os.Exit(1)
		}

		if len(metaNodes) == 0 || len(dataNodes) == 0 {
			fmt.Println("Error: both metadata and data nodes must be specified")
			os.Exit(1)
		}

		if err := deployBackends(); err != nil {
			fmt.Printf("Error deploying backends: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	deployCmd.Flags().Uint32SliceVarP(&metaNodes, "meta-nodes", "", []uint32{}, "Comma-separated list of node IDs for metadata ZDBs")
	deployCmd.Flags().Uint32SliceVarP(&dataNodes, "data-nodes", "", []uint32{}, "Comma-separated list of node IDs for data ZDBs")
	rootCmd.AddCommand(deployCmd)
}

func deployBackends() error {
	// Create grid client
	grid, err := deployer.NewTFPluginClient(Mnemonic, "wss://relay."+Network+".grid.tf", Network)
	if err != nil {
		return errors.Wrap(err, "failed to create grid client")
	}

	// Deploy metadata ZDBs
	metaDeployments := make([]*workloads.ZDB, len(metaNodes))
	for i, nodeID := range metaNodes {
		ns := fmt.Sprintf("meta-%d", nodeID)
		zdb := workloads.ZDB{
			Name:        ns,
			Password:    "zdbpassword",
			Public:      false,
			Size:        1, // 1GB
			Description: "QSFS metadata namespace",
			Mode:        workloads.ZDBModeUser,
		}

		dl := workloads.NewDeployment(fmt.Sprintf("meta-%d", nodeID), nodeID, "", nil, nil, []workloads.ZDB{zdb}, nil, nil, nil)
		if err := grid.DeploymentDeployer.Deploy(context.Background(), dl); err != nil {
			return errors.Wrapf(err, "failed to deploy metadata ZDB on node %d", nodeID)
		}

		metaDeployments[i] = &zdb
		fmt.Printf("Deployed metadata ZDB '%s' on node %d\n", ns, nodeID)
	}

	// Deploy data ZDBs
	dataDeployments := make([]*workloads.ZDB, len(dataNodes))
	for i, nodeID := range dataNodes {
		ns := fmt.Sprintf("data-%d", nodeID)
		zdb := workloads.ZDB{
			Name:        ns,
			Password:    "zdbpassword",
			Public:      false,
			Size:        10, // 10GB
			Description: "QSFS data namespace",
			Mode:        workloads.ZDBModeSeq,
		}

		dl := workloads.NewDeployment(fmt.Sprintf("data-%d", nodeID), nodeID, "", nil, nil, []workloads.ZDB{zdb}, nil, nil, nil)
		if err := grid.DeploymentDeployer.Deploy(context.Background(), dl); err != nil {
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
