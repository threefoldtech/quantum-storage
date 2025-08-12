package cmd

import (
	"bytes"
	"embed"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/threefoldtech/quantum-storage/quantumd/internal/config"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/hook"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/service"

	"github.com/spf13/cobra"
)

const (
	zdbfsVersion = "0.1.11"
	zdbVersion   = "2.0.8"
	zstorVersion = "0.5.0-rc.1"
)

var (
	// localMode is a flag for the setup command
	localMode bool
	// TemplateAssets are embedded files
	TemplateAssets embed.FS
)

// SetAssets populates the embedded file systems
func SetAssets(templates embed.FS) {
	TemplateAssets = templates
	service.TemplateAssets = templates
}

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Setup QSFS components",
	Long: `Downloads binaries and configures services for zstor, zdb and zdbfs.
With --local flag, sets up a complete local test environment with backend ZDBs.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := SetupQSFS(localMode); err != nil {
			fmt.Printf("Error setting up QSFS: %v\n", err)
			os.Exit(1)
		}
	},
}

var startCmd = &cobra.Command{
	Use:   "start [service]",
	Short: "Start a single service",
	Long:  `Starts a single QSFS service (zdb, zstor, zdbfs, quantumd).`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		serviceName := args[0]
		if err := startService(serviceName); err != nil {
			fmt.Printf("Error starting service %s: %v\n", serviceName, err)
			os.Exit(1)
		}
		fmt.Printf("Service %s started successfully.\n", serviceName)
	},
}

func init() {
	setupCmd.Flags().BoolVarP(&localMode, "local", "l", false, "Setup local test environment with backend ZDBs")
	rootCmd.AddCommand(setupCmd)
	rootCmd.AddCommand(startCmd)
}

func SetupQSFS(isLocal bool) error {
	cfg, err := LoadConfig(rootCmd.Flag("config").Value.String())
	if err != nil {
		// In local mode, a config file is not strictly required.
		// We can proceed with a default config.
		if isLocal && os.IsNotExist(err) {
			cfg = &config.Config{
				QsfsMountpoint: "/mnt/qsfs",
				ZdbRootPath:    "/var/lib/zdb",
				CachePath:      "/var/cache/zdbfs",
				Password:       "zdbpassword",
				MinShards:      2,
				ExpectedShards: 4,
			}
		} else {
			return fmt.Errorf("failed to load config: %w", err)
		}
	}

	if err := DownloadBinaries(); err != nil {
		return fmt.Errorf("failed to download binaries: %w", err)
	}

	if err := hook.SetupSymlink(); err != nil {
		return fmt.Errorf("failed to setup hook symlink: %w", err)
	}

	zdbDirExists, err := CreateDirectories(cfg, isLocal)
	if err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	if zdbDirExists {
		isEmpty, err := IsEmpty(cfg.ZdbRootPath)
		if err != nil {
			return fmt.Errorf("failed to check if zdb root path is empty: %w", err)
		}
		if !isEmpty {
			fmt.Printf("WARNING: zdb root path %s is not empty, existing data may be used\n", cfg.ZdbRootPath)
		}
	}

	if isLocal {
		if err := generateLocalZstorConfig(); err != nil {
			return fmt.Errorf("failed to generate local zstor config: %w", err)
		}
	}

	// The service config needs to be converted to the one in the service package
	serviceCfg := &service.Config{
		Network:        cfg.Network,
		Mnemonic:       cfg.Mnemonic,
		DeploymentName: cfg.DeploymentName,
		MetaNodes:      cfg.MetaNodes,
		DataNodes:      cfg.DataNodes,
		Password:       cfg.Password,
		MetaSizeGb:     cfg.MetaSizeGb,
		DataSizeGb:     cfg.DataSizeGb,
		MinShards:      cfg.MinShards,
		ExpectedShards: cfg.ExpectedShards,
		ZdbRootPath:    cfg.ZdbRootPath,
		QsfsMountpoint: cfg.QsfsMountpoint,
		CachePath:      cfg.CachePath,
		RetryInterval:  cfg.RetryInterval,
		DatabasePath:   cfg.DatabasePath,
		ZdbRotateTime:  cfg.ZdbRotateTime,
		ZdbDataSize:    cfg.ZdbDataSize,
		ZdbfsSize:      cfg.ZdbfsSize,
		MetaBackends:   []service.Backend{},
		DataBackends:   []service.Backend{},
	}

	if err := service.Setup(serviceCfg, isLocal); err != nil {
		return err
	}

	sm, err := service.NewServiceManager()
	if err != nil {
		return fmt.Errorf("failed to get service manager: %w", err)
	}

	servicesToStart := []string{"zdb", "zstor", "zdbfs", "quantumd"}
	if isLocal {
		servicesToStart = append([]string{"zdb-back1", "zdb-back2", "zdb-back3", "zdb-back4"}, servicesToStart...)
	}

	for _, srv := range servicesToStart {
		fmt.Printf("Enabling and starting service %s...\n", srv)
		if err := sm.EnableService(srv); err != nil {
			fmt.Printf("warn: failed to enable service %s: %v\n", srv, err)
		}
		if err := sm.StartService(srv); err != nil {
			return fmt.Errorf("failed to start service %s: %w", srv, err)
		}
	}

	if isLocal {
		time.Sleep(2 * time.Second) // Give time for backends to start
		if err := initNamespaces(); err != nil {
			return fmt.Errorf("failed to initialize local namespaces: %w", err)
		}
	}

	fmt.Println("Setup completed successfully.")
	return nil
}

func startService(name string) error {
	sm, err := service.NewServiceManager()
	if err != nil {
		return fmt.Errorf("failed to get service manager: %w", err)
	}
	return sm.StartService(name)
}

func generateLocalZstorConfig() error {
	fmt.Println("Generating local zstor config...")
	cfg, err := LoadConfig(rootCmd.Flag("config").Value.String())
	if err != nil {
		// Fallback to a default if config is not present
		cfg = &config.Config{}
	}

	size, err := parseSize(cfg.ZdbDataSize)
	if err != nil {
		return fmt.Errorf("failed to parse zdb_data_size for local config: %w", err)
	}
	zdbDataSizeMb := size / (1024 * 1024)

	tmpl, err := template.ParseFS(TemplateAssets, "templates/zstor.local.conf.template")
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	var tpl bytes.Buffer
	data := struct {
		MaxZdbDataDirSize uint64
	}{
		MaxZdbDataDirSize: zdbDataSizeMb,
	}

	if err := tmpl.Execute(&tpl, data); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	return os.WriteFile("/etc/zstor.toml", tpl.Bytes(), 0644)
}

func initNamespaces() error {
	for i := 1; i <= 4; i++ {
		port := 9900 + i
		// Create data namespace
		if err := exec.Command("redis-cli", "-p", fmt.Sprint(port), "NSNEW", fmt.Sprintf("data%d", i)).Run(); err != nil {
			fmt.Printf("warn: failed to create data namespace on port %d: %v\n", port, err)
		}
		if err := exec.Command("redis-cli", "-p", fmt.Sprint(port), "NSSET", fmt.Sprintf("data%d", i), "password", "zdbpassword").Run(); err != nil {
			fmt.Printf("warn: failed to set password for data namespace on port %d: %v\n", port, err)
		}
		if err := exec.Command("redis-cli", "-p", fmt.Sprint(port), "NSSET", fmt.Sprintf("data%d", i), "mode", "seq").Run(); err != nil {
			fmt.Printf("warn: failed to set mode for data namespace on port %d: %v\n", port, err)
		}

		// Create meta namespace
		if err := exec.Command("redis-cli", "-p", fmt.Sprint(port), "NSNEW", fmt.Sprintf("meta%d", i)).Run(); err != nil {
			fmt.Printf("warn: failed to create meta namespace on port %d: %v\n", port, err)
		}
		if err := exec.Command("redis-cli", "-p", fmt.Sprint(port), "NSSET", fmt.Sprintf("meta%d", i), "password", "zdbpassword").Run(); err != nil {
			fmt.Printf("warn: failed to set password for meta namespace on port %d: %v\n", port, err)
		}
	}
	return nil
}

func getBinaryVersion(binaryPath string) (string, error) {
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		return "", fmt.Errorf("binary not found")
	}

	cmd := exec.Command(binaryPath, "--version")
	output, err := cmd.CombinedOutput()

	if err != nil && !strings.Contains(err.Error(), "exit status") {
		return "", fmt.Errorf("failed to get version: %w", err)
	}

	outputStr := string(output)

	cleanVersion := func(s string) string {
		re := regexp.MustCompile(`\x1b\[[0-9;]*m`)
		s = re.ReplaceAllString(s, "")
		s = strings.TrimSpace(s)
		s = strings.Trim(s, "'\"[]")
		return s
	}

	if strings.Contains(binaryPath, "zdb") && !strings.Contains(binaryPath, "zdbfs") {
		if strings.Contains(outputStr, "v") {
			parts := strings.Split(outputStr, "v")
			if len(parts) > 1 {
				versionPart := strings.Split(parts[1], " ")[0]
				return cleanVersion(strings.TrimPrefix(versionPart, "v")), nil
			}
		}
	} else if strings.Contains(binaryPath, "zdbfs") {
		if strings.Contains(outputStr, "zdbfs v") {
			parts := strings.Split(outputStr, "zdbfs v")
			if len(parts) > 1 {
				versionPart := strings.Split(parts[1], "\n")[0]
				return cleanVersion(versionPart), nil
			}
		}
	} else if strings.Contains(binaryPath, "zstor") {
		parts := strings.Fields(outputStr)
		if len(parts) >= 2 {
			return cleanVersion(strings.TrimPrefix(parts[1], "v")), nil
		}
	}

	return "", fmt.Errorf("unable to parse version from output: %s", outputStr)
}

func needsDownload(binaryName, expectedVersion string) (bool, error) {
	binaryPath := "/usr/local/bin/" + binaryName

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
func DownloadBinaries() error {
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

func CreateDirectories(cfg *config.Config, localMode bool) (bool, error) {
	dirs := []string{
		cfg.QsfsMountpoint,
		"/var/log",
	}

	_, err := os.Stat(cfg.ZdbRootPath)
	zdbDirExists := !os.IsNotExist(err)

	dirs = append(dirs, cfg.ZdbRootPath)

	if localMode {
		for i := 1; i <= 4; i++ {
			dirs = append(dirs, fmt.Sprintf("/data/data%d", i), fmt.Sprintf("/data/index%d", i))
		}
	}

	for _, dir := range dirs {
		fmt.Printf("Creating directory %s...\n", dir)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return false, fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}
	return zdbDirExists, nil
}

func IsEmpty(name string) (bool, error) {
	f, err := os.Open(name)
	if err != nil {
		return false, err
	}
	defer f.Close()

	_, err = f.Readdirnames(1)
	if err == io.EOF {
		return true, nil
	}
	return false, err
}
