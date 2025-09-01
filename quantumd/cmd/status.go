package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/zstor"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the current status of zdb backends",
	Long:  `This command shows the current status of all zdb backends by querying the zstor prometheus endpoint.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Load config to get zstor config path
		cfg, err := loadDaemonConfig(cmd)
		if err != nil {
			return err
		}
		
		// Initialize zstor metrics scraper
		metricsScraper, err := zstor.NewMetricsScraper(cfg.ZstorConfigPath)
		if err != nil {
			return fmt.Errorf("failed to initialize zstor metrics scraper: %w", err)
		}

		// Scrape metrics
		if err := metricsScraper.ScrapeMetrics(); err != nil {
			return fmt.Errorf("failed to scrape zstor metrics: %w", err)
		}

		// Print status information
		fmt.Println("ZDB Backend Status:")
		fmt.Println("===================")

		if err := printBackendStatus(metricsScraper); err != nil {
			return fmt.Errorf("failed to print backend status: %w", err)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func printBackendStatus(scraper *zstor.MetricsScraper) error {
	statuses := scraper.GetBackendStatuses()

	if len(statuses) == 0 {
		fmt.Println("No backend statuses found.")
		return nil
	}

	// Create a new tabwriter
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	// Print header
	fmt.Fprintln(w, "ADDRESS\tTYPE\tNAMESPACE\tSTATUS\tLAST SEEN")
	fmt.Fprintln(w, "-------\t----\t---------\t------\t---------")

	// Print each backend status
	for _, status := range statuses {
		statusText := "DEAD"
		if status.IsAlive {
			statusText = "ALIVE"
		}

		lastSeen := "Never"
		if !status.LastSeen.IsZero() {
			lastSeen = status.LastSeen.Format("2006-01-02 15:04:05")
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			status.Address,
			status.BackendType,
			status.Namespace,
			statusText,
			lastSeen)
	}

	// Flush the writer to ensure all data is written
	return w.Flush()
}
