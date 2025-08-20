package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/service"
)

var startAll bool

var startCmd = &cobra.Command{
	Use:   "start [service]",
	Short: "Start a single service or all services",
	Long:  `Starts a single QSFS service (zdb, zstor, zdbfs, quantumd) or all services with the --all flag.`, 
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
			if startAll {
				service.StartAllServices()
			} else {
				if len(args) == 0 {
					fmt.Println("Please specify a service to start or use the --all flag.")
					os.Exit(1)
				}
			serviceName := args[0]
			if err := service.StartServiceByName(serviceName); err != nil {
				fmt.Printf("Error starting service %s: %v\n", serviceName, err)
				os.Exit(1)
			}
		}
	},
}

func init() {
	startCmd.Flags().BoolVarP(&startAll, "all", "a", false, "Start all services")
	rootCmd.AddCommand(startCmd)
}
