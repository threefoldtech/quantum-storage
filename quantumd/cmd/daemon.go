package cmd

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/config"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/hook"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/util"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/zstor"
)

func init() {
	rootCmd.AddCommand(daemonCmd)
	daemonCmd.Flags().BoolP("local", "l", false, "Enable local mode for the daemon")
}

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run the main quantumd daemon",
	Long:  `This command starts the quantumd daemon, which manages QSFS components.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		log.Println("Quantum Daemon starting...")

		cfg, err := loadDaemonConfig(cmd)
		if err != nil {
			return err
		}

		zstorClient, err := zstor.NewClient("/usr/local/bin/zstor", cfg.ZstorConfigPath)
		if err != nil {
			return fmt.Errorf("failed to initialize zstor client: %w", err)
		}

		// Initialize zstor metrics scraper
		metricsScraper, err := zstor.NewMetricsScraper(cfg.ZstorConfigPath)
		if err != nil {
			return fmt.Errorf("failed to initialize zstor metrics scraper: %w", err)
		}

		// Create daemon instance
		daemon, err := newDaemon(cfg, zstorClient, metricsScraper)
		if err != nil {
			return fmt.Errorf("failed to initialize daemon: %w", err)
		}

		// Start all goroutines
		go daemon.startHookHandler()
		go daemon.startRetryLoop()
		go daemon.startPrometheusServer()
		go daemon.startMetricsScraper()
		go daemon.startMetadataRefresh()

		// Run main loop
		daemon.run()

		return nil
	},
}

func startPrometheusServer(port int) {
	http.Handle("/metrics", promhttp.Handler())
	addr := fmt.Sprintf(":%d", port)
	log.Printf("Prometheus server listening on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Failed to start Prometheus server: %v", err)
	}
}

func loadDaemonConfig(cmd *cobra.Command) (*config.Config, error) {
	if localMode, _ := cmd.Flags().GetBool("local"); localMode {
		if _, err := os.Stat(ConfigFile); err == nil {
			return config.LoadConfig(ConfigFile)
		}
		// Return a default config for local mode if no config file is present
		return &config.Config{
			QsfsMountpoint: "/mnt/qsfs",
			ZdbRootPath:    "/var/lib/zdb",
			CachePath:      "/var/cache/zdbfs",
			Password:       "zdbpassword",
			MinShards:      2,
			ExpectedShards: 4,
			RetryInterval:  defaultRetryInterval,
		}, nil
	}
	// In remote mode, config is required
	return config.LoadConfig(ConfigFile)
}

// Daemon represents the main daemon structure
type Daemon struct {
	cfg            *config.Config
	zstorClient    *zstor.Client
	metricsScraper *zstor.MetricsScraper

	// In-memory metadata store
	metadataStore map[string]zstor.Metadata
	metadataMutex sync.RWMutex

	// Pending uploads list
	pendingUploads map[string]bool
	pendingMutex   sync.RWMutex

	// Prometheus metrics
	lastRetryRunTime prometheus.Gauge

	// Channels for communication
	hookChan         chan string
	retryChan        chan bool
	uploadCompleteCh chan uploadResult
	metricsChan      chan bool
	metadataChan     chan map[string]zstor.Metadata

	// Channels for internal communication
	quitChan chan bool
}

// uploadResult represents the result of an upload operation
type uploadResult struct {
	filePath string
	metadata *zstor.Metadata
	err      error
}

// newDaemon creates a new daemon instance
func newDaemon(cfg *config.Config, zstorClient *zstor.Client, metricsScraper *zstor.MetricsScraper) (*Daemon, error) {
	lastRetryRunTime := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "last_retry_run_time",
			Help: "The timestamp of the last successful retry cycle.",
		},
	)
	prometheus.MustRegister(lastRetryRunTime)

	d := &Daemon{
		cfg:              cfg,
		zstorClient:      zstorClient,
		metricsScraper:   metricsScraper,
		metadataStore:    make(map[string]zstor.Metadata),
		pendingUploads:   make(map[string]bool),
		lastRetryRunTime: lastRetryRunTime,
		hookChan:         make(chan string, 100),
		retryChan:        make(chan bool, 1),
		uploadCompleteCh: make(chan uploadResult, 100),
		metricsChan:      make(chan bool, 1),
		metadataChan:     make(chan map[string]zstor.Metadata, 1),
		quitChan:         make(chan bool),
	}

	// Initialize metadata store
	if err := d.refreshMetadata(); err != nil {
		return nil, fmt.Errorf("failed to initialize metadata: %w", err)
	}

	return d, nil
}

// refreshMetadata fetches all metadata and updates the in-memory store
func (d *Daemon) refreshMetadata() error {
	log.Println("Refreshing metadata...")

	// Get eligible files
	eligibleFiles, err := util.GetEligibleZdbFiles(d.cfg.ZdbRootPath)
	if err != nil {
		return fmt.Errorf("failed to get eligible files: %w", err)
	}

	// Fetch all metadata
	allMetadata, err := zstor.GetAllMetadata(d.cfg.ZstorConfigPath)
	if err != nil {
		return fmt.Errorf("failed to fetch all metadata: %w", err)
	}

	// Assign filenames to metadata
	filenameMetadata, err := zstor.AssignFilenamesToMetadata(eligibleFiles, allMetadata, d.cfg.ZdbRootPath)
	if err != nil {
		return fmt.Errorf("failed to assign filenames to metadata: %w", err)
	}

	// Update in-memory store
	d.metadataMutex.Lock()
	d.metadataStore = filenameMetadata
	d.metadataMutex.Unlock()

	log.Printf("Metadata refreshed, found metadata for %d files", len(filenameMetadata))
	return nil
}

// startHookHandler starts the hook handler
func (d *Daemon) startHookHandler() {
	handler, err := hook.NewHandler(d.cfg.ZdbRootPath, d.zstorClient)
	if err != nil {
		log.Fatalf("Failed to initialize hook handler: %v", err)
	}
	handler.ListenAndServe()
}

// startRetryLoop starts the retry loop
func (d *Daemon) startRetryLoop() {
	interval := d.cfg.RetryInterval
	if interval <= 0 {
		interval = defaultRetryInterval
	}

	// Run once immediately
	d.retryChan <- true

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			d.retryChan <- true
		case <-d.quitChan:
			return
		}
	}
}

// startPrometheusServer starts the Prometheus metrics server
func (d *Daemon) startPrometheusServer() {
	startPrometheusServer(d.cfg.PrometheusPort)
}

// startMetricsScraper starts the zstor metrics scraper
func (d *Daemon) startMetricsScraper() {
	log.Println("Starting zstor metrics scraper...")

	// Run once immediately
	if err := d.metricsScraper.ScrapeMetrics(); err != nil {
		log.Printf("Failed to scrape zstor metrics: %v", err)
	} else {
		d.metricsChan <- true
	}

	// Then run every 30 seconds
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := d.metricsScraper.ScrapeMetrics(); err != nil {
				log.Printf("Failed to scrape zstor metrics: %v", err)
			} else {
				log.Println("Successfully scraped zstor metrics")
				d.metricsChan <- true
			}
		case <-d.quitChan:
			return
		}
	}
}

// startMetadataRefresh starts the metadata refresh loop
func (d *Daemon) startMetadataRefresh() {
	// Refresh metadata every 5 minutes
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Fetch metadata in background and send to channel
			go func() {
				// Get eligible files
				eligibleFiles, err := util.GetEligibleZdbFiles(d.cfg.ZdbRootPath)
				if err != nil {
					log.Printf("Failed to get eligible files: %v", err)
					return
				}

				// Fetch all metadata
				allMetadata, err := zstor.GetAllMetadata(d.cfg.ZstorConfigPath)
				if err != nil {
					log.Printf("Failed to fetch all metadata: %v", err)
					return
				}

				// Assign filenames to metadata
				filenameMetadata, err := zstor.AssignFilenamesToMetadata(eligibleFiles, allMetadata, d.cfg.ZdbRootPath)
				if err != nil {
					log.Printf("Failed to assign filenames to metadata: %v", err)
					return
				}

				// Send to channel
				d.metadataChan <- filenameMetadata
			}()
		case <-d.quitChan:
			return
		}
	}
}

// run is the main loop of the daemon
func (d *Daemon) run() {
	for {
		select {
		case hookMsg := <-d.hookChan:
			d.handleHookMessage(hookMsg)
		case <-d.retryChan:
			d.handleRetry()
		case result := <-d.uploadCompleteCh:
			d.handleUploadResult(result)
		case <-d.metricsChan:
			d.handleMetricsUpdate()
		case metadata := <-d.metadataChan:
			d.handleMetadataUpdate(metadata)
		case <-d.quitChan:
			log.Println("Daemon shutting down...")
			return
		}
	}
}

// handleHookMessage processes a hook message
func (d *Daemon) handleHookMessage(msg string) {
	log.Printf("Received hook message: %s", msg)
	// TODO: Implement hook message handling
	// This would parse the message and trigger appropriate actions
}

// handleRetry processes the retry loop
func (d *Daemon) handleRetry() {
	log.Println("Running retry cycle...")

	// Get eligible files
	eligibleFiles, err := util.GetEligibleZdbFiles(d.cfg.ZdbRootPath)
	if err != nil {
		log.Printf("Failed to get eligible files: %v", err)
		return
	}

	// Check each eligible file
	for _, filePath := range eligibleFiles {
		// Skip if upload is pending
		if d.isUploadPending(filePath) {
			continue
		}

		// Get local hash
		localHash := zstor.GetLocalHash(filePath)
		if localHash == "" {
			log.Printf("Failed to get local hash for file %s, skipping", filePath)
			continue
		}

		// Check if file exists locally
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			continue
		}

		// Check metadata to see if file is already stored
		d.metadataMutex.RLock()
		_, exists := d.metadataStore[filePath]
		d.metadataMutex.RUnlock()

		// If metadata doesn't exist, upload the file
		if !exists {
			log.Printf("File %s needs upload, queuing...", filePath)
			// Determine if it's an index file
			isIndex := strings.Contains(filePath, "/index/")
			d.uploadFile(filePath, isIndex)
		}
	}

	// Send metrics update
	d.metricsChan <- true
}

// handleUploadResult processes the result of an upload operation
func (d *Daemon) handleUploadResult(result uploadResult) {
	d.pendingMutex.Lock()
	delete(d.pendingUploads, result.filePath)
	d.pendingMutex.Unlock()

	if result.err != nil {
		log.Printf("Upload failed for %s: %v", result.filePath, result.err)
		return
	}

	log.Printf("Upload succeeded for %s", result.filePath)

	// Update metadata store with new metadata
	if result.metadata != nil {
		d.metadataMutex.Lock()
		d.metadataStore[result.filePath] = *result.metadata
		d.metadataMutex.Unlock()
	}
}

// handleMetricsUpdate processes a metrics update
func (d *Daemon) handleMetricsUpdate() {
	timestamp := float64(time.Now().Unix())
	d.lastRetryRunTime.Set(timestamp)
	log.Println("Updated last_retry_run_time metric.")
}

// handleMetadataUpdate processes a metadata update
func (d *Daemon) handleMetadataUpdate(metadata map[string]zstor.Metadata) {
	d.metadataMutex.Lock()
	d.metadataStore = metadata
	d.metadataMutex.Unlock()
	log.Println("Metadata updated")
}

// isUploadPending checks if an upload is pending for a file
func (d *Daemon) isUploadPending(filePath string) bool {
	d.pendingMutex.RLock()
	defer d.pendingMutex.RUnlock()
	return d.pendingUploads[filePath]
}

// markUploadPending marks a file as having a pending upload
func (d *Daemon) markUploadPending(filePath string) bool {
	d.pendingMutex.Lock()
	defer d.pendingMutex.Unlock()

	if d.pendingUploads[filePath] {
		return false // Already pending
	}

	d.pendingUploads[filePath] = true
	return true
}

// uploadFile uploads a file in the background
func (d *Daemon) uploadFile(filePath string, isIndex bool) {
	// Mark as pending
	if !d.markUploadPending(filePath) {
		log.Printf("Upload already pending for %s, skipping", filePath)
		return
	}

	// Start upload in background
	go func() {
		var err error
		var metadata *zstor.Metadata

		if isIndex {
			// Use StoreBatch for all index files to ensure atomicity and correct pathing.
			err = d.zstorClient.StoreBatch([]string{filePath}, filepath.Dir(filePath))
		} else {
			// Use the simplified Store for data files.
			err = d.zstorClient.Store(filePath)
		}

		if err != nil {
			d.uploadCompleteCh <- uploadResult{
				filePath: filePath,
				err:      err,
			}
			return
		}

		// Fetch metadata for the uploaded file
		metadata, err = zstor.GetMetadata(d.cfg.ZstorConfigPath, filePath)
		if err != nil {
			d.uploadCompleteCh <- uploadResult{
				filePath: filePath,
				err:      fmt.Errorf("failed to fetch metadata after upload: %w", err),
			}
			return
		}

		d.uploadCompleteCh <- uploadResult{
			filePath: filePath,
			metadata: metadata,
			err:      nil,
		}
	}()
}
