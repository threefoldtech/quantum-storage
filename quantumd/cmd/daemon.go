package cmd

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/config"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/daemon"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/zstor"
)

func init() {
	rootCmd.AddCommand(daemonCmd)
}

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run the main quantumd daemon",
	Long:  `This command starts the quantumd daemon, which manages QSFS components.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		log.Println("Quantum Daemon starting...")

		cfg, err := config.LoadConfig(ConfigFile)
		if err != nil {
			return err
		}

		zstorClient, err := zstor.NewClient(cfg.ZstorConfigPath)
		if err != nil {
			return fmt.Errorf("failed to initialize zstor client: %w", err)
		}

		// Initialize zstor metrics scraper
		metricsScraper, err := zstor.NewMetricsScraper(cfg.ZstorConfigPath)
		if err != nil {
			return fmt.Errorf("failed to initialize zstor metrics scraper: %w", err)
		}

		// Create daemon instance
		d, err := daemon.NewDaemon(cfg, zstorClient, metricsScraper)
		if err != nil {
			return fmt.Errorf("failed to initialize daemon: %w", err)
		}

		// Initialize metadata store
		if err := d.RefreshMetadata(); err != nil {
			return fmt.Errorf("failed to initialize metadata: %w", err)
		}

		// Start all goroutines
		go d.StartHookHandler()
		go d.StartRetryLoop()
		go d.StartPrometheusServer()
		go d.StartMetricsScraper()
		go d.StartMetadataRefresh()

		// Run main loop
		d.Run()

		return nil
	},
}
