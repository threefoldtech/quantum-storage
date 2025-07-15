package cmd

import (
	"bufio"
	"database/sql"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/spf13/cobra"
)

const (
	defaultRetryInterval = 10 * time.Minute
	uploadedDataFiles    = "uploaded_data_files"
)

var rootCmd = &cobra.Command{
	Use:   "quantum-daemon",
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
					ZdbPassword:    "zdbpassword",
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

		handler, err := newHookHandler()
		if err != nil {
			return fmt.Errorf("failed to initialize hook handler: %w", err)
		}

		dbPath := cfg.DatabasePath
		if dbPath == "" {
			dataDir := filepath.Dir(handler.zstorData)
			dbPath = filepath.Join(dataDir, "uploaded_files.db")
		}

		retryManager, err := newRetryManager(handler, cfg.RetryInterval, dbPath)
		if err != nil {
			return fmt.Errorf("failed to initialize retry manager: %w", err)
		}

		// Run the hook listener in a goroutine
		go handler.listenAndServeHooks()

		// Run the retry manager in a goroutine
		go retryManager.start()

		// Wait indefinitely
		select {}
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&ConfigFile, "config", "c", "/etc/quantumd/config.yaml", "Path to YAML config file")
	rootCmd.PersistentFlags().BoolVarP(&localMode, "local", "l", false, "Enable local mode")
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
)

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// --- Hook Handling Logic ---

const (
	zdbfsPrefixEnv = "ZDBFS_PREFIX"
	defaultPrefix  = "/"
)

type hookHandler struct {
	prefix     string
	zstorConf  string
	zstorBin   string
	zstorIndex string
	zstorData  string
}

func newHookHandler() (*hookHandler, error) {
	prefix := os.Getenv(zdbfsPrefixEnv)
	if prefix == "" {
		prefix = defaultPrefix
	}

	h := &hookHandler{
		prefix:     prefix,
		zstorConf:  filepath.Join(prefix, "etc", "zstor.toml"),
		zstorBin:   filepath.Join(prefix, "bin", "zstor"),
		zstorIndex: filepath.Join(prefix, "data", "index"),
		zstorData:  filepath.Join(prefix, "data", "data"),
	}

	// Verify that the zstor binary exists
	if _, err := os.Stat(h.zstorBin); os.IsNotExist(err) {
		return nil, fmt.Errorf("zstor binary not found at %s", h.zstorBin)
	}

	return h, nil
}

func (h *hookHandler) listenAndServeHooks() {
	// Ensure the socket doesn't already exist
	if err := os.RemoveAll(socketPath); err != nil {
		log.Fatalf("Failed to remove existing socket: %v", err)
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		log.Fatalf("Failed to listen on unix socket %s: %v", socketPath, err)
	}
	defer listener.Close()

	log.Printf("Daemon listening for hooks on %s", socketPath)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Error accepting connection: %v", err)
			continue
		}
		// Handle each connection in a new goroutine to allow concurrent hooks
		go h.handleConnection(conn)
	}
}

func (h *hookHandler) handleConnection(conn net.Conn) {
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	if scanner.Scan() {
		line := scanner.Text()
		log.Printf("Received hook message: %s", line)

		parts := strings.Fields(line)
		if len(parts) == 0 {
			log.Println("Received empty hook message, ignoring.")
			return
		}

		action := parts[0]
		args := parts[1:]

		if err := h.dispatchHook(action, args); err != nil {
			log.Printf("Error handling hook action '%s': %v", action, err)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Error reading from hook connection: %v", err)
	}
}

func (h *hookHandler) dispatchHook(action string, args []string) error {
	log.Printf("Dispatching action: %s with args: %v", action, args)

	// Most actions have at least one arg (instance name)
	// but we check inside the handler where it's relevant.
	switch action {
	case "close":
		return h.handleClose()
	case "ready":
		return h.handleReady()
	case "namespace-created", "namespace-updated":
		if len(args) < 2 {
			return fmt.Errorf("not enough arguments for %s", action)
		}
		return h.handleNamespaceUpdate(args[1]) // arg[0] is instance, arg[1] is namespace
	case "jump-index":
		if len(args) < 4 {
			return fmt.Errorf("not enough arguments for %s", action)
		}
		// The shell script's $5 corresponds to args[3] here.
		// The shell `for` loop splits by whitespace, so we do the same.
		dirtyList := strings.Fields(args[3])
		return h.handleJumpIndex(args[1], dirtyList)
	case "jump-data":
		if len(args) < 2 {
			return fmt.Errorf("not enough arguments for %s", action)
		}
		return h.handleJumpData(args[1]) // arg[0] is instance, arg[1] is data-file-path
	case "missing-data":
		if len(args) < 2 {
			return fmt.Errorf("not enough arguments for %s", action)
		}
		return h.handleMissingData(args[1])
	default:
		return fmt.Errorf("unknown hook action received: %s", action)
	}
}

// runZstor executes the zstor command with the given arguments,
// retrying on failure indefinitely.
func (h *hookHandler) runZstor(args ...string) error {
	for {
		cmd := exec.Command(h.zstorBin, args...)
		log.Printf("Executing: %s", cmd.String())

		output, err := cmd.CombinedOutput()
		if err == nil {
			log.Printf("Successfully executed: %s", cmd.String())
			log.Printf("Output: %s", string(output))
			return nil
		}

		log.Printf("Command failed: %s. Error: %v. Output: %s", cmd.String(), err, string(output))
		log.Println("Retrying in 1 second...")
		time.Sleep(1 * time.Second)
	}
}

func (h *hookHandler) handleClose() error {
	namespaces, err := os.ReadDir(h.zstorData)
	if err != nil {
		return fmt.Errorf("could not read zstor data dir %s: %w", h.zstorData, err)
	}

	for _, ns := range namespaces {
		nsName := ns.Name()
		if !ns.IsDir() || nsName == "zdbfs-temp" {
			continue
		}

		log.Printf("Processing 'close' for namespace: %s", nsName)
		indexDir := filepath.Join(h.zstorIndex, nsName)
		dataDir := filepath.Join(h.zstorData, nsName)

		// Find the last active file number
		indexFiles, err := os.ReadDir(indexDir)
		if err != nil {
			log.Printf("Could not read index dir %s: %v. Skipping.", indexDir, err)
			continue
		}

		lastActive := -1
		for _, f := range indexFiles {
			if strings.HasPrefix(f.Name(), "i") {
				numPart := strings.TrimPrefix(f.Name(), "i")
				num, err := strconv.Atoi(numPart)
				if err == nil && num > lastActive {
					lastActive = num
				}
			}
		}

		if lastActive == -1 {
			log.Printf("No active index files found for namespace %s. Skipping.", nsName)
			continue
		}

		dataFile := filepath.Join(dataDir, fmt.Sprintf("d%d", lastActive))
		indexFile := filepath.Join(indexDir, fmt.Sprintf("i%d", lastActive))

		// Upload data file
		go h.runZstor("-c", h.zstorConf, "store", "-s", "--file", dataFile)
		// Upload index file
		go h.runZstor("-c", h.zstorConf, "store", "-s", "--file", indexFile)
	}
	return nil
}

func (h *hookHandler) handleReady() error {
	// The script runs this in a loop, so we do the same.
	// This runs in the foreground of the hook handler, blocking this hook until ready.
	return h.runZstor("-c", h.zstorConf, "test")
}

func (h *hookHandler) handleNamespaceUpdate(namespace string) error {
	if namespace == "zdbfs-temp" {
		log.Println("Skipping temporary namespace zdbfs-temp")
		return nil
	}
	file := filepath.Join(h.zstorIndex, namespace, "zdb-namespace")
	// Run in a goroutine to not block the hook call
	go h.runZstor("-c", h.zstorConf, "store", "-s", "--file", file)
	return nil
}

func (h *hookHandler) handleJumpIndex(indexPath string, dirtyIndices []string) error {
	namespace := filepath.Base(filepath.Dir(indexPath))
	if namespace == "zdbfs-temp" {
		log.Println("Skipping temporary namespace zdbfs-temp")
		return nil
	}

	// Create a temporary directory to stage the files for upload
	tmpDir, err := os.MkdirTemp("/tmp", "zdb.hook.tmp.XXXXXXXX")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	// defer os.RemoveAll(tmpDir) // The zstor command might need this to persist

	dirBase := filepath.Dir(indexPath)

	// Copy dirty index files
	for _, dirty := range dirtyIndices {
		fileName := fmt.Sprintf("i%s", dirty)
		src := filepath.Join(dirBase, fileName)
		dst := filepath.Join(tmpDir, fileName)
		if err := copyFile(src, dst); err != nil {
			log.Printf("Failed to copy dirty index file %s: %v", src, err)
			continue // Try to upload what we can
		}
	}

	// Copy the main index file that triggered the jump
	if err := copyFile(indexPath, filepath.Join(tmpDir, filepath.Base(indexPath))); err != nil {
		return fmt.Errorf("failed to copy main index file %s: %w", indexPath, err)
	}

	// Upload the entire directory in the background
	go h.runZstor("-c", h.zstorConf, "store", "-s", "-d", "-f", tmpDir, "-k", dirBase)

	return nil
}

func (h *hookHandler) handleJumpData(dataPath string) error {
	namespace := filepath.Base(filepath.Dir(dataPath))
	if namespace == "zdbfs-temp" {
		log.Println("Skipping temporary namespace zdbfs-temp")
		return nil
	}
	// Run in a goroutine to not block the hook call
	go h.runZstor("-c", h.zstorConf, "store", "-s", "--file", dataPath)
	return nil
}

func (h *hookHandler) handleMissingData(dataPath string) error {
	// This needs to be synchronous, as zdb is waiting for the file.
	return h.runZstor("-c", h.zstorConf, "retrieve", "--file", dataPath)
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
	handler       *hookHandler
	interval      time.Duration
	uploadTracker *uploadTracker
}

func (ut *uploadTracker) MigrateFromTextFile(textFilePath string) error {
	if _, err := os.Stat(textFilePath); os.IsNotExist(err) {
		// No text file to migrate
		return nil
	}

	content, err := os.ReadFile(textFilePath)
	if err != nil {
		return fmt.Errorf("failed to read text file: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	migrated := 0

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		filePath := parts[0]
		hash := parts[1]

		// Get file size if file exists
		var fileSize int64
		if info, err := os.Stat(filePath); err == nil {
			fileSize = info.Size()
		}

		// Insert into database
		if err := ut.MarkUploaded(filePath, hash, fileSize); err != nil {
			log.Printf("Failed to migrate %s: %v", filePath, err)
			continue
		}
		migrated++
	}

	if migrated > 0 {
		log.Printf("Migrated %d entries from text file to SQLite", migrated)

		// Optionally backup the old text file
		backupPath := textFilePath + ".backup"
		if err := os.Rename(textFilePath, backupPath); err != nil {
			log.Printf("Failed to backup text file: %v", err)
		}
	}

	return nil
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

func newRetryManager(handler *hookHandler, interval time.Duration, dbPath string) (*retryManager, error) {
	uploadTracker, err := newUploadTracker(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create upload tracker: %w", err)
	}

	// Migrate from old text file if it exists
	dataDir := filepath.Dir(handler.zstorData)
	textFilePath := filepath.Join(dataDir, uploadedDataFiles)
	if err := uploadTracker.MigrateFromTextFile(textFilePath); err != nil {
		log.Printf("Failed to migrate from text file: %v", err)
		// Continue anyway, this is not fatal
	}

	if interval <= 0 {
		interval = defaultRetryInterval
	}

	return &retryManager{
		handler:       handler,
		interval:      interval,
		uploadTracker: uploadTracker,
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
	namespaces, err := os.ReadDir(rm.handler.zstorData)
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
	namespaceFile := filepath.Join(rm.handler.zstorIndex, namespace, "zdb-namespace")
	rm.checkAndUploadFile(namespaceFile)

	// Process data and index files
	for _, fileType := range []string{"data", "index"} {
		rm.processFiles(namespace, fileType, tmpDir)
	}
}

func (rm *retryManager) processFiles(namespace, fileType, tmpDir string) {
	var basePath string
	if fileType == "data" {
		basePath = filepath.Join(rm.handler.zstorData, namespace)
	} else {
		basePath = filepath.Join(rm.handler.zstorIndex, namespace)
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
			rm.checkAndUploadFile(tmpPath)
		} else {
			// Check if data file was already uploaded
			uploaded, err := rm.uploadTracker.IsUploaded(file)
			if err != nil {
				log.Printf("Failed to check upload status for %s: %v", file, err)
				continue
			}
			if !uploaded {
				rm.checkAndUploadFile(file)
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
func (rm *retryManager) checkAndUploadFile(file string) {
	if _, err := os.Stat(file); os.IsNotExist(err) {
		return
	}

	// Get remote and local hashes
	remoteHash := rm.getRemoteHash(file)
	localHash := rm.getLocalHash(file)

	if localHash == "" {
		log.Printf("Failed to get local hash for %s", file)
		return
	}

	// Store file if hashes don't match or remote check failed
	if remoteHash == "" || remoteHash != localHash {
		log.Printf("Uploading %s (remote: %s, local: %s)", file, remoteHash, localHash)

		// Use a single attempt version of runZstor for retry manager
		cmd := exec.Command(rm.handler.zstorBin, "-c", rm.handler.zstorConf, "store", "-s", "--file", file)
		log.Printf("Executing: %s", cmd.String())

		output, err := cmd.CombinedOutput()
		if err != nil {
			log.Printf("Failed to upload %s: %v. Output: %s", file, err, string(output))
			return
		}

		log.Printf("Successfully uploaded: %s", file)

		// Track uploaded data files
		if strings.Contains(file, "/data/") {
			rm.markDataFileUploaded(file, localHash)
		}
	} else if remoteHash == localHash && strings.Contains(file, "/data/") {
		// Already uploaded, mark it
		rm.markDataFileUploaded(file, localHash)
	}
}
func (rm *retryManager) getRemoteHash(file string) string {
	cmd := exec.Command(rm.handler.zstorBin, "-c", rm.handler.zstorConf, "check", "--file", file)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func (rm *retryManager) getLocalHash(file string) string {
	// Try b2sum first, fallback to sha256sum
	cmd := exec.Command("b2sum", "-l", "128", file)
	output, err := cmd.Output()
	if err != nil {
		// Fallback to sha256sum
		cmd = exec.Command("sha256sum", file)
		output, err = cmd.Output()
		if err != nil {
			return ""
		}
	}
	parts := strings.Fields(string(output))
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

func (rm *retryManager) markDataFileUploaded(file, hash string) {
	fileInfo, err := os.Stat(file)
	if err != nil {
		log.Printf("Failed to get file info for %s: %v", file, err)
		return
	}

	if err := rm.uploadTracker.MarkUploaded(file, hash, fileInfo.Size()); err != nil {
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

const socketPath = "/tmp/zdb-hook.sock"
