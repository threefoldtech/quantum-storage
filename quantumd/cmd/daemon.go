package cmd

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/config"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/hook"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/zstor"
)

var (
	lastRetryRunTime = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "last_retry_run_time",
			Help: "The timestamp of the last successful retry cycle.",
		},
	)
)

func init() {
	prometheus.MustRegister(lastRetryRunTime)
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

		dbPath := cfg.DatabasePath
		if dbPath == "" {
			dbPath = filepath.Join(cfg.ZdbRootPath, "uploaded_files.db")
		}

		uploadTracker, err := newUploadTracker(dbPath)
		if err != nil {
			return fmt.Errorf("failed to initialize upload tracker: %w", err)
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

		handler, err := hook.NewHandler(cfg.ZdbRootPath, uploadTracker, zstorClient)
		if err != nil {
			return fmt.Errorf("failed to initialize hook handler: %w", err)
		}

		retryManager, err := newRetryManager(cfg, uploadTracker, zstorClient)
		if err != nil {
			return fmt.Errorf("failed to initialize retry manager: %w", err)
		}

		go handler.ListenAndServe()
		go retryManager.start()
		go startPrometheusServer(cfg.PrometheusPort)
		go startMetricsScraper(metricsScraper)

		select {}
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

func init() {
	rootCmd.AddCommand(daemonCmd)
	daemonCmd.Flags().BoolP("local", "l", false, "Enable local mode for the daemon")
}

type uploadTracker struct {
	db *sql.DB
}

type retryManager struct {
	zdbRootPath   string
	interval      time.Duration
	uploadTracker *uploadTracker
	zstor         *zstor.Client
}

func newUploadTracker(dbPath string) (*uploadTracker, error) {
	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	schema := `
	CREATE TABLE IF NOT EXISTS uploaded_files (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		file_path TEXT NOT NULL UNIQUE,
		hash TEXT NOT NULL,
		file_size INTEGER,
		uploaded_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_file_path ON uploaded_files(file_path);
	CREATE INDEX IF NOT EXISTS idx_hash ON uploaded_files(hash);
	`
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create database schema: %w", err)
	}

	return &uploadTracker{db: db}, nil
}

func (ut *uploadTracker) Close() error {
	if ut.db != nil {
		return ut.db.Close()
	}
	return nil
}

func (ut *uploadTracker) IsUploaded(filePath string) (bool, error) {
	var count int
	err := ut.db.QueryRow("SELECT COUNT(*) FROM uploaded_files WHERE file_path = ?", filePath).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (ut *uploadTracker) MarkUploaded(filePath, hash string, fileSize int64) error {
	_, err := ut.db.Exec(`
		INSERT OR REPLACE INTO uploaded_files (file_path, hash, file_size, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
	`, filePath, hash, fileSize)
	return err
}

func (ut *uploadTracker) GetUploadedFileHash(filePath string) (string, error) {
	var hash string
	err := ut.db.QueryRow("SELECT hash FROM uploaded_files WHERE file_path = ?", filePath).Scan(&hash)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil // Not found is not an error, just return empty hash
		}
		return "", err
	}
	return hash, nil
}

func (ut *uploadTracker) MarkUploadedBatch(filesToUpdate map[string]string) error {
	tx, err := ut.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() // Rollback on error

	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO uploaded_files (file_path, hash, file_size, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for filePath, hash := range filesToUpdate {
		var fileSize int64
		if info, err := os.Stat(filePath); err == nil {
			fileSize = info.Size()
		}

		if _, err := stmt.Exec(filePath, hash, fileSize); err != nil {
			return fmt.Errorf("failed to mark '%s' as uploaded in batch: %w", filePath, err)
		}
	}

	return tx.Commit()
}

func newRetryManager(cfg *config.Config, tracker *uploadTracker, zstorClient *zstor.Client) (*retryManager, error) {
	interval := cfg.RetryInterval
	if interval <= 0 {
		interval = defaultRetryInterval
	}

	return &retryManager{
		zdbRootPath:   cfg.ZdbRootPath,
		interval:      interval,
		uploadTracker: tracker,
		zstor:         zstorClient,
	}, nil
}

func (rm *retryManager) start() {
	log.Println("Starting retry manager...")
	defer rm.uploadTracker.Close()

	// Run once immediately, since the loop will start after one tick
	rm.runRetryCycle()

	ticker := time.NewTicker(rm.interval)
	defer ticker.Stop()

	for range ticker.C {
		rm.runRetryCycle()
	}
}

func (rm *retryManager) runRetryCycle() {
	log.Println("Running retry cycle...")

	zstorDataPath := filepath.Join(rm.zdbRootPath, "data")
	namespaces, err := os.ReadDir(zstorDataPath)
	if err != nil {
		log.Printf("Failed to read zstor data dir: %v", err)
		return
	}

	for _, ns := range namespaces {
		if !ns.IsDir() || ns.Name() == "zdbfs-temp" || ns.Name() == "default" {
			continue
		}
		rm.processNamespace(ns.Name())
	}

	rm.sendMetrics()
}

func (rm *retryManager) processNamespace(namespace string) {
	log.Printf("Processing namespace: %s", namespace)

	zstorIndexPath := filepath.Join(rm.zdbRootPath, "index")
	namespaceFile := filepath.Join(zstorIndexPath, namespace, "zdb-namespace")
	rm.checkAndUploadFile(namespaceFile, true)

	for _, fileType := range []string{"data", "index"} {
		rm.processFiles(namespace, fileType)
	}
}

func (rm *retryManager) processFiles(namespace, fileType string) {
	var basePath string
	var isIndex bool
	if fileType == "data" {
		basePath = filepath.Join(rm.zdbRootPath, "data", namespace)
		isIndex = false
	} else {
		basePath = filepath.Join(rm.zdbRootPath, "index", namespace)
		isIndex = true
	}

	prefix := fileType[:1]
	files, err := filepath.Glob(filepath.Join(basePath, prefix+"*"))
	if err != nil {
		log.Printf("Failed to list %s files: %v", fileType, err)
		return
	}

	if len(files) <= 1 {
		return
	}

	sort.Slice(files, func(i, j int) bool {
		return extractNumber(files[i]) < extractNumber(files[j])
	})

	filesToProcess := files[:len(files)-1]

	if isIndex {
		// For index files, check local cache and upload only what's needed.
		var filesToUpload []string
		filesToMark := make(map[string]string)

		for _, file := range filesToProcess {
			localHash := zstor.GetLocalHash(file)
			if localHash == "" {
				log.Printf("Failed to get local hash for index file %s, skipping", file)
				continue
			}

			storedHash, err := rm.uploadTracker.GetUploadedFileHash(file)
			if err != nil {
				log.Printf("Failed to check local cache for index file %s: %v", file, err)
				continue
			}

			if localHash != storedHash {
				log.Printf("Index file %s needs upload (stored: '%s', local: '%s')", file, storedHash, localHash)
				filesToUpload = append(filesToUpload, file)
				filesToMark[file] = localHash
			}
		}

		if len(filesToUpload) > 0 {
			log.Printf("Batch uploading %d out-of-sync index files for namespace %s", len(filesToUpload), namespace)
			if err := rm.zstor.StoreBatch(filesToUpload, basePath); err != nil {
				log.Printf("Failed to batch upload index files for namespace %s: %v", namespace, err)
			} else {
				// Mark them as uploaded in the local cache after successful upload
				if err := rm.uploadTracker.MarkUploadedBatch(filesToMark); err != nil {
					log.Printf("Failed to mark batch of index files as uploaded for namespace %s: %v", namespace, err)
				}
			}
		}
	} else {
		// Process data files individually
		for _, file := range filesToProcess {
			uploaded, err := rm.uploadTracker.IsUploaded(file)
			if err != nil {
				log.Printf("Failed to check upload status for %s: %v", file, err)
				continue
			}
			if !uploaded {
				rm.checkAndUploadFile(file, false)
			}
		}
	}
}

func (rm *retryManager) checkAndUploadFile(file string, isIndex bool) {
	if _, err := os.Stat(file); os.IsNotExist(err) {
		return
	}

	remoteHash, err := rm.zstor.Check(file)
	if err != nil {
		log.Printf("Failed to get remote hash for %s: %v", file, err)
		return
	}

	localHash := zstor.GetLocalHash(file)
	if localHash == "" {
		log.Printf("Failed to get local hash for %s", file)
		return
	}

	if remoteHash == "" || remoteHash != localHash {
		log.Printf("Uploading %s (remote: %s, local: %s)", file, remoteHash, localHash)
		var uploadErr error
		if isIndex {
			// Use StoreBatch for all index files to ensure atomicity and correct pathing.
			uploadErr = rm.zstor.StoreBatch([]string{file}, filepath.Dir(file))
		} else {
			// Use the simplified Store for data files.
			uploadErr = rm.zstor.Store(file)
		}

		if uploadErr != nil {
			log.Printf("Failed to upload %s: %v", file, uploadErr)
			return
		}
		log.Printf("Successfully uploaded: %s", file)
	}

	if !isIndex {
		rm.markDataFileUploaded(file, localHash)
	}
}

func (rm *retryManager) markDataFileUploaded(file, hash string) {
	fileInfo, err := os.Stat(file)
	if err != nil {
		log.Printf("Could not stat file %s, assuming it was uploaded and removed: %v", file, err)
		if err := rm.uploadTracker.MarkUploaded(file, hash, 0); err != nil {
			log.Printf("Failed to mark file as uploaded after stat error: %v", err)
		}
		return
	}

	if err := rm.uploadTracker.MarkUploaded(file, hash, fileInfo.Size()); err != nil {
		log.Printf("Failed to mark file as uploaded: %v", err)
	}
}

func extractNumber(filename string) int {
	base := filepath.Base(filename)
	if len(base) < 2 {
		return 0
	}
	numStr := base[1:]
	num, err := strconv.Atoi(numStr)
	if err != nil {
		return 0
	}
	return num
}

func (rm *retryManager) sendMetrics() {
	timestamp := float64(time.Now().Unix())
	lastRetryRunTime.Set(timestamp)
	log.Println("Updated last_retry_run_time metric.")
}

// startMetricsScraper starts the zstor metrics scraper that periodically fetches
// backend connection status from the zstor prometheus endpoint
func startMetricsScraper(scraper *zstor.MetricsScraper) {
	log.Println("Starting zstor metrics scraper...")

	// Run once immediately
	if err := scraper.ScrapeMetrics(); err != nil {
		log.Printf("Failed to scrape zstor metrics: %v", err)
	}

	// Then run every 30 seconds
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if err := scraper.ScrapeMetrics(); err != nil {
			log.Printf("Failed to scrape zstor metrics: %v", err)
		} else {
			log.Println("Successfully scraped zstor metrics")
		}
	}
}
