package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Setup QSFS components",
	Long:  `Downloads binaries and configures services for zstor, zdb and zdbfs.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := setupQSFS(); err != nil {
			fmt.Printf("Error setting up QSFS: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
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
	for name := range services {
		cmd := exec.Command("zinit", "monitor", name)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to monitor service %s: %w", name, err)
		}
	}

	return nil
}
