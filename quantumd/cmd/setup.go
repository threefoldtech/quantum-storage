package cmd

import (
	"embed"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/scottyeager/tfgrid-sdk-go/grid-client/workloads"
	"github.com/spf13/cobra"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/config"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/hook"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/service"
)

const (
	zdbfsVersion = "0.1.11"
	zdbVersion   = "2.0.8"
	zstorVersion = "0.5.0-rc.1"
)

var (
	// TemplateAssets are embedded files
	TemplateAssets embed.FS
)

func DownloadBinaries() error {
	var quantumdVersion string
	// Get quantumd version for metadata decoder. We use the same version since
	// these are released together
	if version != "dev" {
		quantumdVersion = strings.TrimPrefix(version, "v")
	} else {
		quantumdVersion = "dev"
	}

	binaries := map[string]string{
		"zdbfs":                  fmt.Sprintf("https://github.com/threefoldtech/0-db-fs/releases/download/v%s/zdbfs-%s-amd64-linux-static", zdbfsVersion, zdbfsVersion),
		"zdb":                    fmt.Sprintf("https://github.com/threefoldtech/0-db/releases/download/v%s/zdb-%s-linux-amd64-static", zdbVersion, zdbVersion),
		"zstor":                  fmt.Sprintf("https://github.com/threefoldtech/0-stor_v2/releases/download/v%s/zstor_v2-x86_64-linux-musl", zstorVersion),
		"zstor-metadata-decoder": fmt.Sprintf("https://github.com/threefoldtech/quantum-storage/releases/download/v%s/zstor-metadata-decoder_linux_amd64", quantumdVersion),
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
		case "zstor-metadata-decoder":
			expectedVersion = quantumdVersion
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

	// If quantumd is a dev build, we can't download the corresponding metadata
	// decoder version. For dev testing, we provide our own decoder binary
	if currentVersion == "dev" {
		fmt.Printf("Binary %s has dev version, skipping download\n", binaryName)
		return false, nil
	}

	if currentVersion == expectedVersion {
		fmt.Printf("Binary %s already has correct version %s, skipping download\n", binaryName, expectedVersion)
		return false, nil
	}

	fmt.Printf("Binary %s has version %s, expected %s, will download\n", binaryName, currentVersion, expectedVersion)
	return true, nil
}

// SetAssets populates the embedded file systems
func SetAssets(templates embed.FS) {
	TemplateAssets = templates
	service.TemplateAssets = templates
}

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Setup QSFS components",
	Long:  `Downloads binaries and configures services for zstor, zdb and zdbfs.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := SetupQSFS(); err != nil {
			fmt.Printf("Error setting up QSFS: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(setupCmd)
}

func SetupQSFS() error {
	cfg, err := config.LoadConfig(rootCmd.Flag("config").Value.String())
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if err := DownloadBinaries(); err != nil {
		return fmt.Errorf("failed to download binaries: %w", err)
	}

	if err := hook.SetupSymlink(); err != nil {
		return fmt.Errorf("failed to setup hook symlink: %w", err)
	}

	zdbDirExists, err := CreateDirectories(cfg)
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

	// The service config needs to be converted to the one in the service package
	if err := service.Setup(cfg, []workloads.Deployment{}, []workloads.Deployment{}); err != nil {
		return err
	}

	sm, err := service.NewServiceManager()
	if err != nil {
		return fmt.Errorf("failed to get service manager: %w", err)
	}

	servicesToStart := []string{"zdb", "zstor", "zdbfs", "quantumd"}
	for _, srv := range servicesToStart {
		fmt.Printf("Enabling and starting service %s...\n", srv)
		if err := sm.EnableService(srv); err != nil {
			fmt.Printf("warn: failed to enable service %s: %v\n", srv, err)
		}
		if err := sm.StartService(srv); err != nil {
			return fmt.Errorf("failed to start service %s: %w", srv, err)
		}
	}

	fmt.Println("Setup completed successfully.")
	return nil
}

func CreateDirectories(cfg *config.Config) (bool, error) {
	dirs := []string{
		cfg.QsfsMountpoint,
		"/var/log",
		cfg.ZdbRootPath,
	}

	_, err := os.Stat(cfg.ZdbRootPath)
	zdbDirExists := !os.IsNotExist(err)

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
	} else if strings.Contains(binaryPath, "zstor") && !strings.Contains(binaryPath, "zstor-metadata-decoder") {
		parts := strings.Fields(outputStr)
		if len(parts) >= 2 {
			return cleanVersion(strings.TrimPrefix(parts[1], "v")), nil
		}
	} else if strings.Contains(binaryPath, "zstor-metadata-decoder") {
		parts := strings.Fields(outputStr)
		if len(parts) >= 2 {
			return cleanVersion(strings.TrimPrefix(parts[1], "v")), nil
		}
	}

	return "", fmt.Errorf("unable to parse version from output: %s", outputStr)
}
