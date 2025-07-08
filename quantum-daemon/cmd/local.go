package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var localCmd = &cobra.Command{
	Use:   "local",
	Short: "Setup local test environment with backend ZDBs",
	Long: `Sets up a complete local test environment with:
- 1 frontend ZDB
- 4 backend ZDBs 
- zstor configured to use them
- zdbfs mounted`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := setupLocal(); err != nil {
			fmt.Printf("Error setting up local mode: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(localCmd)
}

func setupLocal() error {
	// Create directories for backend ZDBs
	for i := 1; i <= 4; i++ {
		dirs := []string{
			fmt.Sprintf("/data/data%d", i),
			fmt.Sprintf("/data/index%d", i),
		}
		for _, dir := range dirs {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", dir, err)
			}
		}
	}

	// Start backend ZDBs
	for i, port := range []int{9901, 9902, 9903, 9904} {
		cmd := exec.Command(
			"/usr/local/bin/zdb",
			fmt.Sprintf("--port=%d", port),
			fmt.Sprintf("--data=/data/data%d", i+1),
			fmt.Sprintf("--index=/data/index%d", i+1),
			fmt.Sprintf("--logfile=/var/log/zdb%d.log", i+1),
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("failed to start zdb backend %d: %w", i+1, err)
		}
	}

	// Initialize namespaces (similar to docker/setup.sh)
	if err := initNamespaces(); err != nil {
		return err
	}

	// Generate local zstor config
	if err := generateLocalConfig(); err != nil {
		return err
	}

	// Proceed with normal setup
	return setupQSFS()
}

func initNamespaces() error {
	for port := 9901; port <= 9904; port++ {
		// Data namespace
		if err := exec.Command("redis-cli", "-p", fmt.Sprint(port), "NSNEW", fmt.Sprintf("data%d", port-9900)).Run(); err != nil {
			return fmt.Errorf("failed to create data namespace on port %d: %w", port, err)
		}
		if err := exec.Command("redis-cli", "-p", fmt.Sprint(port), "NSSET", fmt.Sprintf("data%d", port-9900), "password", "zdbpassword").Run(); err != nil {
			return err
		}
		if err := exec.Command("redis-cli", "-p", fmt.Sprint(port), "NSSET", fmt.Sprintf("data%d", port-9900), "mode", "seq").Run(); err != nil {
			return err
		}

		// Meta namespace  
		if err := exec.Command("redis-cli", "-p", fmt.Sprint(port), "NSNEW", fmt.Sprintf("meta%d", port-9900)).Run(); err != nil {
			return fmt.Errorf("failed to create meta namespace on port %d: %w", port, err)
		}
		if err := exec.Command("redis-cli", "-p", fmt.Sprint(port), "NSSET", fmt.Sprintf("meta%d", port-9900), "password", "zdbpassword").Run(); err != nil {
			return err
		}
	}
	return nil
}

func generateLocalConfig() error {
	// Similar to docker/zstor_config.toml but with local paths
	config := `minimal_shards = 2
expected_shards = 4
redundant_groups = 0
redundant_nodes = 0
root = "/"
zdbfs_mountpoint = "/mnt/qsfs/"
socket = "/tmp/zstor.sock"
prometheus_port = 9200
zdb_data_dir_path = "/data/data/zdbfs-data/"
max_zdb_data_dir_size = 64

[compression]
algorithm = "snappy"

[meta]
type = "zdb"

[meta.config]
prefix = "zstor-meta"

[encryption]
algorithm = "AES"
key = "5ee8ab34dbd57df581d9aada2e433f3fae7e55833f9350fa74dfe196d0f5240f"

[meta.config.encryption]
algorithm = "AES"
key = "5ee8ab34dbd57df581d9aada2e433f3fae7e55833f9350fa74dfe196d0f5240f"

[[meta.config.backends]]
address = "127.0.0.1:9901"
namespace = "meta1"
password = "zdbpassword"

[[meta.config.backends]]
address = "127.0.0.1:9902"
namespace = "meta2"
password = "zdbpassword"

[[meta.config.backends]]
address = "127.0.0.1:9903"
namespace = "meta3"
password = "zdbpassword"

[[meta.config.backends]]
address = "127.0.0.1:9904"
namespace = "meta4"
password = "zdbpassword"

[[groups]]
[[groups.backends]]
address = "127.0.0.1:9901"
namespace = "data1"
password = "zdbpassword"

[[groups.backends]]
address = "127.0.0.1:9902"
namespace = "data2"
password = "zdbpassword"

[[groups.backends]]
address = "127.0.0.1:9903"
namespace = "data3"
password = "zdbpassword"

[[groups.backends]]
address = "127.0.0.1:9904"
namespace = "data4"
password = "zdbpassword"`

	return os.WriteFile("/etc/zstor-default.toml", []byte(config), 0644)
}
