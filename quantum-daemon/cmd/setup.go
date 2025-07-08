package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	localMode bool
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Setup QSFS components",
	Long: `Downloads binaries and configures services for zstor, zdb and zdbfs.
With --local flag, sets up a complete local test environment with backend ZDBs.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := setupQSFS(); err != nil {
			fmt.Printf("Error setting up QSFS: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	setupCmd.Flags().BoolVarP(&localMode, "local", "l", false, "Setup local test environment with backend ZDBs")
	rootCmd.AddCommand(setupCmd)
}

func setupQSFS() error {
	// Check if systemd or zinit is available
	initSystem, err := detectInitSystem()
	if err != nil {
		return fmt.Errorf("failed to detect init system: %w", err)
	}

	fmt.Printf("Detected init system: %s\n", initSystem)

	// Download binaries
	if err := downloadBinaries(); err != nil {
		return fmt.Errorf("failed to download binaries: %w", err)
	}

	// Create directories
	if err := createDirectories(); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	if localMode {
		// Setup backend ZDBs for local mode
		if err := setupLocalBackends(initSystem); err != nil {
			return fmt.Errorf("failed to setup local backends: %w", err)
		}

		// Generate local config
		if err := generateLocalConfig(); err != nil {
			return err
		}
	}

	// Setup services based on init system
	switch initSystem {
	case "systemd":
		return setupSystemdServices()
	case "zinit":
		return setupZinitServices()
	default:
		return fmt.Errorf("unsupported init system: %s", initSystem)
	}
}

func setupLocalBackends(initSystem string) error {
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

	// Setup backend ZDB services
	switch initSystem {
	case "systemd":
		return setupLocalSystemdBackends()
	case "zinit":
		return setupLocalZinitBackends()
	default:
		return fmt.Errorf("unsupported init system for local backends: %s", initSystem)
	}
}

func setupLocalSystemdBackends() error {
	for i, port := range []int{9901, 9902, 9903, 9904} {
		service := fmt.Sprintf(`[Unit]
Description=Local ZDB Backend %d
After=network.target

[Service]
ExecStart=/usr/local/bin/zdb \
    --port=%d \
    --data=/data/data%d \
    --index=/data/index%d \
    --logfile=/var/log/zdb%d.log \
    --datasize 67108864 \
    --hook /usr/local/bin/zdb-hook.sh \
    --rotate 900
Restart=always
RestartSec=5
TimeoutStopSec=60

[Install]
WantedBy=multi-user.target`, i+1, port, i+1, i+1, i+1)

		path := fmt.Sprintf("/etc/systemd/system/zdb-back%d.service", i+1)
		if err := os.WriteFile(path, []byte(service), 0644); err != nil {
			return fmt.Errorf("failed to write service file %s: %w", path, err)
		}

		cmd := exec.Command("systemctl", "daemon-reload")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to reload systemd: %w", err)
		}

		cmd = exec.Command("systemctl", "enable", "--now", fmt.Sprintf("zdb-back%d", i+1))
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to enable service zdb-back%d: %w", i+1, err)
		}
	}
	return initNamespaces()
}

func setupLocalZinitBackends() error {
	for i, port := range []int{9901, 9902, 9903, 9904} {
		service := fmt.Sprintf(`exec: /usr/local/bin/zdb \
    --port=%d \
    --data=/data/data%d \
    --index=/data/index%d \
    --logfile=/var/log/zdb%d.log \
    --datasize 67108864 \
    --hook /usr/local/bin/zdb-hook.sh \
    --rotate 900
shutdown_timeout: 60`, port, i+1, i+1, i+1)

		path := fmt.Sprintf("/etc/zinit/zdb-back%d.yaml", i+1)
		if err := os.WriteFile(path, []byte(service), 0644); err != nil {
			return fmt.Errorf("failed to write service file %s: %w", path, err)
		}

		cmd := exec.Command("zinit", "monitor", fmt.Sprintf("zdb-back%d", i+1))
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to monitor service zdb-back%d: %w", i+1, err)
		}
	}
	return initNamespaces()
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

func detectInitSystem() (string, error) {
	if _, err := exec.LookPath("systemctl"); err == nil {
		return "systemd", nil
	}
	if _, err := exec.LookPath("zinit"); err == nil {
		return "zinit", nil
	}
	return "", fmt.Errorf("no supported init system found")
}

const (
	zdbfsVersion = "0.1.11"
	zdbVersion   = "2.0.8"
	zstorVersion = "0.4.0"
)

func downloadBinaries() error {
	binaries := map[string]string{
		"zdbfs": fmt.Sprintf("https://github.com/threefoldtech/0-db-fs/releases/download/v%s/zdbfs-%s-amd64-linux-static", zdbfsVersion, zdbfsVersion),
		"zdb":   fmt.Sprintf("https://github.com/threefoldtech/0-db/releases/download/v%s/zdb-%s-linux-amd64-static", zdbVersion, zdbVersion),
		"zstor": fmt.Sprintf("https://github.com/threefoldtech/0-stor_v2/releases/download/v%s/zstor_v2-x86_64-linux-musl", zstorVersion),
	}

	for name, url := range binaries {
		fmt.Printf("Downloading %s...\n", name)
		dest := "/usr/local/bin/" + name
		if name == "zstor" {
			dest = "/bin/zstor"
		}

		cmd := exec.Command("wget", "-O", dest, url)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to download %s: %w", name, err)
		}

		if err := os.Chmod(dest, 0755); err != nil {
			return fmt.Errorf("failed to make %s executable: %w", name, err)
		}
	}

	// Download hook script
	hookURL := "https://raw.githubusercontent.com/threefoldtech/quantum-storage/master/lib/zdb-hook.sh"
	hookDest := "/usr/local/bin/zdb-hook.sh"
	fmt.Println("Downloading zdb hook script...")
	cmd := exec.Command("wget", "-O", hookDest, hookURL)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to download hook script: %w", err)
	}
	return os.Chmod(hookDest, 0755)
}

func createDirectories() error {
	dirs := []string{
		"/mnt/qsfs",
		"/data",
		"/var/log",
	}

	for _, dir := range dirs {
		fmt.Printf("Creating directory %s...\n", dir)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}
	return nil
}

func setupSystemdServices() error {
	services := []string{"zstor", "zdb", "zdbfs"}
	
	for _, name := range services {
		content, err := ServiceFiles.ReadFile("assets/systemd/" + name + ".service")
		if err != nil {
			return fmt.Errorf("failed to read embedded service file %s: %w", name, err)
		}

		path := filepath.Join("/etc/systemd/system", name+".service")
		fmt.Printf("Creating systemd service %s...\n", name)
		if err := os.WriteFile(path, content, 0644); err != nil {
			return fmt.Errorf("failed to write service file %s: %w", path, err)
		}
	}

	// Reload systemd and enable services
	cmd := exec.Command("systemctl", "daemon-reload")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to reload systemd: %w", err)
	}

	for _, name := range services {
		cmd := exec.Command("systemctl", "enable", "--now", name)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to enable service %s: %w", name, err)
		}
	}

	return nil
}

func setupZinitServices() error {
	services := []string{"zstor", "zdb", "zdbfs"}
	zinitDir := "/etc/zinit"
	
	if err := os.MkdirAll(zinitDir, 0755); err != nil {
		return fmt.Errorf("failed to create zinit directory: %w", err)
	}

	for _, name := range services {
		content, err := ServiceFiles.ReadFile("assets/zinit/" + name + ".yaml")
		if err != nil {
			return fmt.Errorf("failed to read embedded service file %s: %w", name, err)
		}

		path := filepath.Join(zinitDir, name+".yaml")
		fmt.Printf("Creating zinit service %s...\n", name)
		if err := os.WriteFile(path, content, 0644); err != nil {
			return fmt.Errorf("failed to write service file %s: %w", path, err)
		}
	}

	// Start monitoring services
	for _, name := range services {
		cmd := exec.Command("zinit", "monitor", name)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to monitor service %s: %w", name, err)
		}
	}

	return nil
}
