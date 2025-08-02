package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/service"
)

func init() {
	rootCmd.AddCommand(servicesCmd)
}

var servicesCmd = &cobra.Command{
	Use:   "services",
	Short: "List all managed services and their status.",
	Long: `Checks for the existence and running state of all services
that quantumd can manage (e.g., zdb, zstor, zdbfs, quantumd).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		sm, err := service.NewServiceManager()
		if err != nil {
			return fmt.Errorf("failed to get service manager: %w", err)
		}

		fmt.Printf("%-15s %-10s %-10s\n", "Service", "Exists", "Running")
		for _, s := range service.ManagedServices {
			exists, err := sm.ServiceExists(s)
			if err != nil {
				return fmt.Errorf("failed to check if service %s exists: %w", s, err)
			}

			running, err := sm.ServiceIsRunning(s)
			if err != nil {
				return fmt.Errorf("failed to check if service %s is running: %w", s, err)
			}

			fmt.Printf("%-15s %-10s %-10s\n", s, boolToString(exists), boolToString(running))
		}

		return nil
	},
}

func boolToString(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}