package cmd

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/spf13/cobra"
)

const (
	zdbfsVersion = "0.1.11"
	zdbVersion   = "2.0.8"
	zstorVersion = "0.5.0-rc.1"
)

var (
	localMode      bool
	SystemdAssets  embed.FS
	TemplateAssets embed.FS
)

func SetAssets(systemd, templates embed.FS) {
	SystemdAssets = systemd
	TemplateAssets = templates
}

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

	cfg, err := LoadConfig(rootCmd.Flag("config").Value.String())
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Download binaries
	if err := downloadBinaries(); err != nil {
		return fmt.Errorf("failed to download binaries: %w", err)
	}

	// Create directories
	if err := createDirectories(cfg); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	// Generate zstor config
	if localMode {
		if err := generateLocalZstorConfig(); err != nil {
			return fmt.Errorf("failed to generate local zstor config: %w", err)
		}
	} else {
		if err := generateZstorConfig(cfg); err != nil {
			return fmt.Errorf("failed to generate zstor config: %w", err)
		}
	}

	if localMode {
		// Setup backend ZDBs for local mode
		if err := setupLocalBackends(initSystem); err != nil {
			return fmt.Errorf("failed to setup local backends: %w", err)
		}
	}

	// Setup services based on init system
	switch initSystem {
	case "systemd":
		return setupSystemdServices(cfg)
	case "zinit":
		return setupZinitServices(cfg)
	default:
		return fmt.Errorf("unsupported init system: %s", initSystem)
	}
}

func renderTemplate(destPath, templateName, serviceType string, cfg *Config) error {
	var templatePath string
	if serviceType == "" {
		templatePath = filepath.Join("assets/templates", templateName)
	} else {
		templatePath = filepath.Join("assets/templates", serviceType, templateName)
	}
	templateContent, err := TemplateAssets.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("failed to read embedded template %s: %w", templatePath, err)
	}

	tmpl, err := template.New(templateName).Parse(string(templateContent))
	if err != nil {
		return fmt.Errorf("failed to parse template %s: %w", templateName, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, cfg); err != nil {
		return fmt.Errorf("failed to execute template %s: %w", templateName, err)
	}

	return os.WriteFile(destPath, buf.Bytes(), 0644)
}

func generateZstorConfig(cfg *Config) error {
	fmt.Println("Generating zstor config...")
	// Pass an empty serviceType because zstor.conf.template is in the root of the templates directory
	return renderTemplate("/etc/zstor.toml", "zstor.conf.template", "", cfg)
}

func generateLocalZstorConfig() error {
	fmt.Println("Generating local zstor config...")
	config := `minimal_shards = 2
expected_shards = 4
redundant_groups = 0
redundant_nodes = 0
root = "/"
zdbfs_mountpoint = "/mnt/"
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
password = "zdbpassword"
`
	return os.WriteFile("/etc/zstor.toml", []byte(config), 0644)
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
		service := fmt.Sprintf(`exec: |
  /usr/local/bin/zdb
    --port %d
    --data /data/data%d
    --index /data/index%d
    --logfile /var/log/zdb%d.log`, port, i+1, i+1, i+1)

		path := fmt.Sprintf("/etc/zinit/zdb-back%d.yaml", i+1)
		if err := os.WriteFile(path, []byte(service), 0644); err != nil {
			return fmt.Errorf("failed to write service file %s: %w", path, err)
		}

		cmd := exec.Command("zinit", "monitor", fmt.Sprintf("zdb-back%d", i+1))
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to monitor service zdb-back%d: %w", i+1, err)
		}
	}
	// Wait for ZDB to start up
	time.Sleep(2 * time.Second)
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

func detectInitSystem() (string, error) {
	// First check if systemd is actually running as PID 1
	if _, err := os.Stat("/proc/1/comm"); err == nil {
		if comm, err := os.ReadFile("/proc/1/comm"); err == nil {
			if strings.TrimSpace(string(comm)) == "systemd" {
				return "systemd", nil
			}
		}
	}

	// Fall back to checking for zinit
	if _, err := exec.LookPath("zinit"); err == nil {
		return "zinit", nil
	}

	return "", fmt.Errorf("no supported init system found")
}

func getBinaryVersion(binaryPath string) (string, error) {
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		return "", fmt.Errorf("binary not found")
	}

	cmd := exec.Command(binaryPath, "--version")
	output, err := cmd.CombinedOutput()

	// zdbfs exits with code 1, so ignore that here
	if err != nil && !strings.Contains(err.Error(), "exit status") {
		return "", fmt.Errorf("failed to get version: %w", err)
	}

	outputStr := string(output)

	// Helper function to clean version string
	cleanVersion := func(s string) string {
		// Remove ANSI escape sequences
		re := regexp.MustCompile(`\x1b\[[0-9;]*m`)
		s = re.ReplaceAllString(s, "")
		// Remove other control characters
		s = strings.TrimSpace(s)
		// Remove any trailing quotes or brackets
		s = strings.Trim(s, "'\"[]")
		return s
	}

	// Parse version based on binary type
	if strings.Contains(binaryPath, "zdb") && !strings.Contains(binaryPath, "zdbfs") {
		// zdb format: "0-db engine, v2.0.8 (commit 2.0.8)"
		if strings.Contains(outputStr, "v") {
			parts := strings.Split(outputStr, "v")
			if len(parts) > 1 {
				versionPart := strings.Split(parts[1], " ")[0]
				return cleanVersion(strings.TrimPrefix(versionPart, "v")), nil
			}
		}
	} else if strings.Contains(binaryPath, "zdbfs") {
		// zdbfs format: "[+] initializing zdbfs v0.1.12"
		if strings.Contains(outputStr, "zdbfs v") {
			parts := strings.Split(outputStr, "zdbfs v")
			if len(parts) > 1 {
				versionPart := strings.Split(parts[1], "\n")[0]
				return cleanVersion(versionPart), nil
			}
		}
	} else if strings.Contains(binaryPath, "zstor") {
		// zstor format: "zstor_v2 0.4.0"
		parts := strings.Fields(outputStr)
		if len(parts) >= 2 {
			return cleanVersion(strings.TrimPrefix(parts[1], "v")), nil
		}
	}

	return "", fmt.Errorf("unable to parse version from output: %s", outputStr)
}

func needsDownload(binaryName, expectedVersion string) (bool, error) {
	var binaryPath string
	if binaryName == "zstor" {
		binaryPath = "/bin/zstor"
	} else {
		binaryPath = "/usr/local/bin/" + binaryName
	}

	currentVersion, err := getBinaryVersion(binaryPath)
	if err != nil {
		if strings.Contains(err.Error(), "binary not found") {
			fmt.Printf("Binary %s not found, will download\n", binaryName)
			return true, nil
		}
		fmt.Printf("Error checking %s version: %v, will download\n", binaryName, err)
		return true, nil
	}

	if currentVersion == expectedVersion {
		fmt.Printf("Binary %s already has correct version %s, skipping download\n", binaryName, expectedVersion)
		return false, nil
	}

	fmt.Printf("Binary %s has version %s, expected %s, will download\n", binaryName, currentVersion, expectedVersion)
	return true, nil
}
func downloadBinaries() error {
	binaries := map[string]string{
		"zdbfs": fmt.Sprintf("https://github.com/threefoldtech/0-db-fs/releases/download/v%s/zdbfs-%s-amd64-linux-static", zdbfsVersion, zdbfsVersion),
		"zdb":   fmt.Sprintf("https://github.com/threefoldtech/0-db/releases/download/v%s/zdb-%s-linux-amd64-static", zdbVersion, zdbVersion),
		"zstor": fmt.Sprintf("https://github.com/threefoldtech/0-stor_v2/releases/download/v%s/zstor_v2-x86_64-linux-musl", zstorVersion),
	}

	for name, url := range binaries {
		var expectedVersion string
		switch name {
		case "zdbfs":
			expectedVersion = zdbfsVersion
		case "zdb":
			expectedVersion = zdbVersion
		case "zstor":
			expectedVersion = zstorVersion
		}

		needsDL, err := needsDownload(name, expectedVersion)
		if err != nil {
			return fmt.Errorf("failed to check if %s needs download: %w", name, err)
		}

		if !needsDL {
			continue
		}

		fmt.Printf("Downloading %s v%s...\n", name, expectedVersion)
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

	return nil
}

func createDirectories(cfg *Config) error {
	dirs := []string{
		cfg.QsfsMountpoint,
		cfg.ZdbRootPath,
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

func setupSystemdServices(cfg *Config) error {
	// Template-based services
	templatedServices := []string{"zdb", "zstor", "zdbfs", "quantumd"}
	for _, name := range templatedServices {
		err := renderTemplate(
			fmt.Sprintf("/etc/systemd/system/%s.service", name),
			fmt.Sprintf("%s.service.template", name),
			"systemd",
			cfg,
		)
		if err != nil {
			return err
		}
	}

	// Reload systemd and enable services
	cmd := exec.Command("systemctl", "daemon-reload")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to reload systemd: %w", err)
	}

	allServices := []string{"zdb", "zstor", "zdbfs", "quantumd"}
	for _, name := range allServices {
		cmd := exec.Command("systemctl", "enable", "--now", name)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to enable service %s: %w", name, err)
		}
	}

	return nil
}

func setupZinitServices(cfg *Config) error {
	services := []string{"zstor", "zdb", "zdbfs", "quantumd"}
	zinitDir := "/etc/zinit"

	if err := os.MkdirAll(zinitDir, 0755); err != nil {
		return fmt.Errorf("failed to create zinit directory: %w", err)
	}

	for _, name := range services {
		err := renderTemplate(
			filepath.Join(zinitDir, name+".yaml"),
			name+".yaml.template",
			"zinit",
			cfg,
		)
		if err != nil {
			return err
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
