package cmd

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/spf13/cobra"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/hook"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/zstor"
)

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

		zstorClient, err := zstor.NewClient("/usr/local/bin/zstor", "/etc/zstor.toml")
		if err != nil {
			return fmt.Errorf("failed to initialize zstor client: %w", err)
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

		select {}
	},
}

func loadDaemonConfig(cmd *cobra.Command) (*Config, error) {
	if localMode, _ := cmd.Flags().GetBool("local"); localMode {
		if _, err := os.Stat(ConfigFile); err == nil {
			return LoadConfig(ConfigFile)
		}
		// Return a default config for local mode if no config file is present
		return &Config{
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
	return LoadConfig(ConfigFile)
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
	zstor         hook.ZstorClient
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

func newRetryManager(cfg *Config, tracker *uploadTracker, zstorClient hook.ZstorClient) (*retryManager, error) {
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
		if !ns.IsDir() || ns.Name() == "zdbfs-temp" {
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

	for _, file := range filesToProcess {
		if isIndex {
			rm.checkAndUploadFile(file, true)
		} else {
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
		// For the retry manager, we always use a snapshot for index files.
		if err := rm.zstor.Store(file, isIndex, isIndex); err != nil {
			log.Printf("Failed to upload %s: %v", file, err)
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
	if _, err := exec.LookPath("pgrep"); err != nil {
		return // pgrep not available
	}
	if err := exec.Command("pgrep", "prometheus-push").Run(); err != nil {
		return // Pushgateway not running
	}

	timestamp := time.Now().Unix()
	metrics := fmt.Sprintf("# TYPE last_retry_run_time gauge\nlast_retry_run_time %d\n", timestamp)

	curlCmd := exec.Command("curl", "-s", "--data-binary", "@-", "localhost:9091/metrics/job/qsfs_retry_uploads")
	curlCmd.Stdin = strings.NewReader(metrics)
	if err := curlCmd.Run(); err != nil {
		log.Printf("Failed to send metrics to pushgateway: %v", err)
	}
}
