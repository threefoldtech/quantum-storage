package cmd

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/spf13/cobra"
)

var (
	localMode bool
)

var (
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

	// Load configuration
	configPath := "/etc/quantumd/config.yaml"
	if localMode {
		configPath = "./config.local.yaml" // Or some other local path
		// You might want to generate a default local config here if it doesn’t exist
	}
	cfg, err := LoadConfig(configPath)
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
	if err := generateZstorConfig(cfg); err != nil {
		return fmt.Errorf("failed to generate zstor config: %w", err)
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
