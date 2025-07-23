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
)

const (
	defaultRetryInterval = 10 * time.Minute
)

var rootCmd = &cobra.Command{
	Use:   "quantumd",
	Short: "Quantum Storage Filesystem management daemon",
	Long:  `Automates the setup and management of QSFS components including zstor, zdb and zdbfs.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// This is the main daemon entry point.
		// It runs when no subcommand is specified.

		log.Println("Quantum Daemon starting...")

		var cfg *Config
		var err error

		if localMode {
			// In local mode, config is optional
			if _, err := os.Stat(ConfigFile); err == nil {
				cfg, err = LoadConfig(ConfigFile)
				if err != nil {
					return fmt.Errorf("failed to load config: %w", err)
				}
			} else {
				// Create a default config for local mode
				cfg = &Config{
					QsfsMountpoint: "/mnt/qsfs",
					ZdbRootPath:    "/var/lib/zdb",
					CachePath:      "/var/cache/zdbfs",
					Password:       "zdbpassword",
					MinShards:      2,
					ExpectedShards: 4,
				}
			}
		} else {
			// In remote mode, config is required
			cfg, err = LoadConfig(ConfigFile)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}
		}

		// Determine database path
		dbPath := cfg.DatabasePath
		if dbPath == "" {
			// Default path relative to ZdbRootPath
			dbPath = filepath.Join(cfg.ZdbRootPath, "uploaded_files.db")
		}

		// Initialize a single upload tracker for both hook and retry manager
		uploadTracker, err := newUploadTracker(dbPath)
		if err != nil {
			return fmt.Errorf("failed to initialize upload tracker: %w", err)
		}

		handler, err := hook.NewHandler(cfg.ZdbRootPath, uploadTracker)
		if err != nil {
			return fmt.Errorf("failed to initialize hook handler: %w", err)
		}

		retryManager, err := newRetryManager(handler, cfg.RetryInterval, uploadTracker)
		if err != nil {
			return fmt.Errorf("failed to initialize retry manager: %w", err)
		}

		// Run the hook listener in a goroutine
		go handler.ListenAndServe()

		// Run the retry manager in a goroutine
		go retryManager.start()

		// Wait indefinitely
		select {}
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&ConfigFile, "config", "c", "/etc/quantumd.yaml", "Path to YAML config file")
	rootCmd.PersistentFlags().BoolVarP(&localMode, "local", "l", false, "Enable local mode")
	rootCmd.PersistentFlags().Bool("version", false, "Print the version number of quantumd")
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(checkCmd)

	// Add version flag handler
	rootCmd.PreRun = func(cmd *cobra.Command, args []string) {
		if showVersion, _ := cmd.Flags().GetBool("version"); showVersion {
			fmt.Printf("quantumd version %s\n", version)
			if commit != "" {
				fmt.Printf("commit: %s\n", commit)
			}
			if date != "" {
				fmt.Printf("built at: %s\n", date)
			}
			os.Exit(0)
		}
	}
}

var (
	LocalMode     bool
	Mnemonic      string
	ConfigOutPath string
	Network       = func() string {
		if env := os.Getenv("NETWORK"); env != "" {
			return env
		}
		return "dev" // default to devnet
	}()
	ConfigFile string
	// Version information will be set during build
	version = "dev"
	commit  = ""
	date    = ""
)

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of quantumd",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("quantumd version %s\n", version)
		if commit != "" {
			fmt.Printf("commit: %s\n", commit)
		}
		if date != "" {
			fmt.Printf("built at: %s\n", date)
		}
	},
}

// copyFile utility
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

// uploadTracker handles tracking of uploaded files using SQLite
type uploadTracker struct {
	db *sql.DB
}

// retryManager handles periodic scanning and retrying of failed uploads
type retryManager struct {
	handler       *hook.Handler
	interval      time.Duration
	uploadTracker *uploadTracker
}

func newUploadTracker(dbPath string) (*uploadTracker, error) {
	// Ensure directory exists
	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Open or create database
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Create table if not exists
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
	err := ut.db.QueryRow(
		"SELECT COUNT(*) FROM uploaded_files WHERE file_path = ?",
		filePath,
	).Scan(&count)

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

func (ut *uploadTracker) GetHash(filePath string) (string, error) {
	var hash string
	err := ut.db.QueryRow(
		"SELECT hash FROM uploaded_files WHERE file_path = ?",
		filePath,
	).Scan(&hash)

	if err == sql.ErrNoRows {
		return "", nil
	}
	return hash, err
}

func (ut *uploadTracker) Remove(filePath string) error {
	_, err := ut.db.Exec("DELETE FROM uploaded_files WHERE file_path = ?", filePath)
	return err
}

func (ut *uploadTracker) Count() (int64, error) {
	var count int64
	err := ut.db.QueryRow("SELECT COUNT(*) FROM uploaded_files").Scan(&count)
	return count, err
}

func (ut *uploadTracker) CleanupMissingFiles() (int64, error) {
	// Find all files in database that no longer exist
	rows, err := ut.db.Query("SELECT file_path FROM uploaded_files")
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var missingFiles []string
	for rows.Next() {
		var filePath string
		if err := rows.Scan(&filePath); err != nil {
			continue
		}
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			missingFiles = append(missingFiles, filePath)
		}
	}

	// Remove missing files from database
	deleted := int64(0)
	for _, filePath := range missingFiles {
		if err := ut.Remove(filePath); err == nil {
			deleted++
		}
	}

	return deleted, nil
}

func (ut *uploadTracker) Vacuum() error {
	_, err := ut.db.Exec("VACUUM")
	return err
}

func newRetryManager(handler *hook.Handler, interval time.Duration, tracker *uploadTracker) (*retryManager, error) {
	if interval <= 0 {
		interval = defaultRetryInterval
	}

	return &retryManager{
		handler:       handler,
		interval:      interval,
		uploadTracker: tracker,
	}, nil
}

func (rm *retryManager) start() {
	log.Println("Starting retry manager...")
	defer rm.uploadTracker.Close()

	for {
		rm.runRetryCycle()
		time.Sleep(rm.interval)
	}
}

func (rm *retryManager) runRetryCycle() {
	log.Println("Running retry cycle...")

	// Process each namespace
	namespaces, err := os.ReadDir(rm.handler.ZstorData)
	if err != nil {
		log.Printf("Failed to read zstor data dir: %v", err)
		return
	}

	// Create temp directory for index files
	tmpDir, err := os.MkdirTemp("/tmp", "zdb.retry.tmp.")
	if err != nil {
		log.Printf("Failed to create temp dir: %v", err)
		return
	}
	defer os.RemoveAll(tmpDir)

	for _, ns := range namespaces {
		if !ns.IsDir() || ns.Name() == "zdbfs-temp" {
			continue
		}

		nsName := ns.Name()
		rm.processNamespace(nsName, tmpDir)
	}

	// Send metrics if prometheus pushgateway is available
	rm.sendMetrics()
}

func (rm *retryManager) processNamespace(namespace, tmpDir string) {
	log.Printf("Processing namespace: %s", namespace)

	// Process namespace file
	namespaceFile := filepath.Join(rm.handler.ZstorIndex, namespace, "zdb-namespace")
	rm.checkAndUploadFile(namespaceFile, "")

	// Process data and index files
	for _, fileType := range []string{"data", "index"} {
		rm.processFiles(namespace, fileType, tmpDir)
	}
}

func (rm *retryManager) processFiles(namespace, fileType, tmpDir string) {
	var basePath string
	if fileType == "data" {
		basePath = filepath.Join(rm.handler.ZstorData, namespace)
	} else {
		basePath = filepath.Join(rm.handler.ZstorIndex, namespace)
	}

	prefix := fileType[:1] // "d" for data, "i" for index

	files, err := filepath.Glob(filepath.Join(basePath, prefix+"*"))
	if err != nil {
		log.Printf("Failed to list %s files: %v", fileType, err)
		return
	}

	// Sort files and skip the largest sequence number
	if len(files) <= 1 {
		return
	}

	// Sort files by extracting numeric suffix
	sort.Slice(files, func(i, j int) bool {
		return extractNumber(files[i]) < extractNumber(files[j])
	})

	// Skip the last file (largest sequence number)
	filesToProcess := files[:len(files)-1]

	for _, file := range filesToProcess {
		if fileType == "index" {
			// Copy index files to temp directory to avoid concurrent access issues
			tmpPath := filepath.Join(tmpDir, namespace+"_"+filepath.Base(file))
			if err := copyFile(file, tmpPath); err != nil {
				log.Printf("Failed to copy index file %s: %v", file, err)
				continue
			}
			rm.checkAndUploadFile(tmpPath, file)
		} else {
			// Check if data file was already uploaded
			uploaded, err := rm.uploadTracker.IsUploaded(file)
			if err != nil {
				log.Printf("Failed to check upload status for %s: %v", file, err)
				continue
			}
			if !uploaded {
				rm.checkAndUploadFile(file, "")
			}
		}
	}
}

// extractNumber extracts the numeric suffix from a filename like "d123" -> 123
func extractNumber(filename string) int {
	base := filepath.Base(filename)
	prefix := base[:1]
	if prefix != "d" && prefix != "i" {
		return 0
	}

	numStr := base[1:]
	num, err := strconv.Atoi(numStr)
	if err != nil {
		return 0
	}
	return num
}
func (rm *retryManager) checkAndUploadFile(file, key string) {
	if _, err := os.Stat(file); os.IsNotExist(err) {
		return
	}

	remoteFile := file
	if key != "" {
		remoteFile = key
	}

	// Get remote and local hashes
	remoteHash := rm.getRemoteHash(remoteFile)
	localHash := hook.GetLocalHash(file)

	if localHash == "" {
		log.Printf("Failed to get local hash for %s", file)
		return
	}

	// Store file if hashes don't match or remote check failed
	if remoteHash == "" || remoteHash != localHash {
		log.Printf("Uploading %s (remote: %s, local: %s)", remoteFile, remoteHash, localHash)

		// Use a single attempt version of runZstor for retry manager
		args := []string{"-c", rm.handler.ZstorConf, "store", "-s", "--file", file}
		if key != "" {
			args = append(args, "-k", key)
		}
		cmd := exec.Command(rm.handler.ZstorBin, args...)
		log.Printf("Executing: %s", cmd.String())

		output, err := cmd.CombinedOutput()
		if err != nil {
			log.Printf("Failed to upload %s: %v. Output: %s", remoteFile, err, string(output))
			return
		}

		log.Printf("Successfully uploaded: %s", remoteFile)

		// Track uploaded data files
		if strings.Contains(remoteFile, "/data/") {
			rm.markDataFileUploaded(file, remoteFile, localHash)
		}
	} else if remoteHash == localHash && strings.Contains(remoteFile, "/data/") {
		// Already uploaded, mark it
		rm.markDataFileUploaded(file, remoteFile, localHash)
	}
}
func (rm *retryManager) getRemoteHash(file string) string {
	cmd := exec.Command(rm.handler.ZstorBin, "-c", rm.handler.ZstorConf, "check", "--file", file)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}



func (rm *retryManager) markDataFileUploaded(localFile, remoteFile, hash string) {
	fileInfo, err := os.Stat(localFile)
	if err != nil {
		// The file might get removed by zstor while we're in progress. We
		// should still try to mark the original file as uploaded. We can't get
		// the size, so we'll use 0.
		log.Printf("Could not stat file %s, assuming it was uploaded and removed: %v", localFile, err)
		if err := rm.uploadTracker.MarkUploaded(remoteFile, hash, 0); err != nil {
			log.Printf("Failed to mark file as uploaded after stat error: %v", err)
		}
		return
	}

	if err := rm.uploadTracker.MarkUploaded(remoteFile, hash, fileInfo.Size()); err != nil {
		log.Printf("Failed to mark file as uploaded: %v", err)
	}
}

func (rm *retryManager) sendMetrics() {
	// Check if prometheus pushgateway is running
	cmd := exec.Command("pgrep", "prometheus-push")
	if err := cmd.Run(); err != nil {
		return // Pushgateway not running
	}

	timestamp := time.Now().Unix()
	metrics := fmt.Sprintf(`# TYPE last_retry_run_time gauge
last_retry_run_time %d
`, timestamp)

	curlCmd := exec.Command("curl", "-s", "--data-binary", "@-", "localhost:9091/metrics/job/qsfs_retry_uploads")
	curlCmd.Stdin = strings.NewReader(metrics)
	curlCmd.Run()
}
