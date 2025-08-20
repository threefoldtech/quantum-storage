package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/service"
)

var stopAll bool

func init() {
	stopCmd.Flags().BoolVarP(&stopAll, "all", "a", false, "Stop all services")
	rootCmd.AddCommand(stopCmd)
}

var stopCmd = &cobra.Command{
	Use:   "stop [service]",
	Short: "Stop a single service or all services.",
	Long:  `Stops a single QSFS service (zdb, zstor, zdbfs, quantumd) or all services with the --all flag.`,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if stopAll {
			service.StopAllServices()
			return nil
		}
		if len(args) == 0 {
			return fmt.Errorf("please specify a service to stop or use the --all flag")
		}
		serviceName := args[0]
		return service.StopServiceByName(serviceName)
	},
}
