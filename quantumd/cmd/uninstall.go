package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/config"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/service"
)

func init() {
	rootCmd.AddCommand(uninstallCmd)
	uninstallCmd.Flags().BoolP("force", "f", false, "Force uninstallation without confirmation")
}

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Uninstall QSFS components and services",
	Long: `Stops all running QSFS services, removes them from the init system,
and deletes all related binaries from the system.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		force, _ := cmd.Flags().GetBool("force")
		if !force {
			fmt.Print("Are you sure you want to uninstall all QSFS components? (y/n) ")
			reader := bufio.NewReader(os.Stdin)
			input, _ := reader.ReadString('\n')
			if strings.TrimSpace(strings.ToLower(input)) != "y" {
				fmt.Println("Uninstall operation cancelled.")
				return nil
			}
		}

		fmt.Println("Starting uninstallation process...")

		cfg, err := config.LoadConfig(ConfigFile)
		if err != nil {
			// A missing config is not a fatal error for uninstall, but we can't show the data dir warning.
			fmt.Printf("Warning: could not load config file: %v. Proceeding with uninstall.\n", err)
			cfg = &config.Config{} // Use an empty config
		}

		sm, err := service.NewServiceManager()
		if err != nil {
			// If we can't get a service manager, we can still try to remove files.
			fmt.Printf("Warning: could not get service manager: %v. Will proceed with file removal.\n", err)
		}

		// List of all services to manage, including local backends
		services := append(service.ManagedServices, "zdb-back1", "zdb-back2", "zdb-back3", "zdb-back4")

		if sm != nil {
			fmt.Println("Stopping and disabling services...")
			for _, srv := range services {
				exists, _ := sm.ServiceExists(srv)
				if !exists {
					continue
				}

				fmt.Printf(" - Stopping %s...\n", srv)
				if err := sm.StopService(srv); err != nil {
					fmt.Printf("  - Warning: failed to stop service %s: %v\n", srv, err)
				}

				fmt.Printf(" - Disabling %s...\n", srv)
				if err := sm.DisableService(srv); err != nil {
					fmt.Printf("  - Failed to disable service %s, waiting and retrying: %v\n", srv, err)
					time.Sleep(2 * time.Second)
					if err := sm.DisableService(srv); err != nil {
						fmt.Printf("  - Warning: failed to disable service %s on second attempt: %v\n", srv, err)
					}
				}
			}

			fmt.Println("Reloading init system daemon...")
			if err := sm.DaemonReload(); err != nil {
				fmt.Printf("Warning: failed to reload init system daemon: %v\n", err)
			}
		}

		fmt.Println("Removing service files...")
		// This part is best-effort and attempts to clean up both systemd and zinit files
		// in case the init system was changed or files were left over.
		for _, srv := range services {
			// systemd
			servicePath := fmt.Sprintf("/etc/systemd/system/%s.service", srv)
			if err := removeFileIfExists(servicePath); err != nil {
				fmt.Printf("Warning: failed to remove %s: %v\n", servicePath, err)
			}
			// zinit
			yamlPath := fmt.Sprintf("/etc/zinit/%s.yaml", srv)
			if err := removeFileIfExists(yamlPath); err != nil {
				fmt.Printf("Warning: failed to remove %s: %v\n", yamlPath, err)
			}
		}

		fmt.Println("Removing binaries...")
		binaries := []string{"zdb", "zstor", "zdbfs", "quantumd", "quantumd-hook"}
		for _, bin := range binaries {
			binPath := filepath.Join("/usr/local/bin", bin)
			if err := removeFileIfExists(binPath); err != nil {
				fmt.Printf("Warning: failed to remove binary %s: %v\n", binPath, err)
			}
		}

		fmt.Println("Uninstallation complete.")
		if cfg.ZdbRootPath != "" {
			fmt.Printf("\nNOTE: Data may still be present in the zdb data directory: %s\n", cfg.ZdbRootPath)
			fmt.Println("This directory was not removed to prevent accidental data loss.")
		}
		return nil
	},
}

func removeFileIfExists(path string) error {
	if _, err := os.Stat(path); err == nil {
		fmt.Printf(" - Removing %s\n", path)
		return os.Remove(path)
	} else if !os.IsNotExist(err) {
		return err
	}
	// File does not exist, which is fine.
	return nil
}
