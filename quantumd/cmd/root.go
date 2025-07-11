package cmd

import (
	"bufio"
	"embed"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "quantum-daemon",
	Short: "Quantum Storage Filesystem management daemon",
	Long:  `Automates the setup and management of QSFS components including zstor, zdb and zdbfs.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// This is the main daemon entry point.
		// It runs when no subcommand is specified.

		// For now, we just start the hook listener.
		// Other daemon activities (like periodic retries) can be added here.
		log.Println("Quantum Daemon starting...")

		handler, err := newHookHandler()
		if err != nil {
			return fmt.Errorf("failed to initialize hook handler: %w", err)
		}

		// Run the hook listener in a goroutine so it doesn't block
		go handler.listenAndServeHooks()

		// The main goroutine can perform other tasks or simply wait.
		// For now, we'll just wait indefinitely.
		select {}
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&ConfigFile, "config", "c", "", "Path to YAML config file")
}

type Config struct {
	Network    string   `yaml:"network"`
	Mnemonic   string   `yaml:"mnemonic"`
	MetaNodes  []uint32 `yaml:"meta_nodes"`
	DataNodes  []uint32 `yaml:"data_nodes"`
	ZDBPass    string   `yaml:"zdb_password"`
	MetaSizeGB int      `yaml:"meta_size_gb"`
	DataSizeGB int      `yaml:"data_size_gb"`
}

var (
	LocalMode     bool
	ServiceFiles  embed.FS
	Mnemonic      string
	ConfigOutPath string
	Network       = func() string {
		if env := os.Getenv("NETWORK"); env != "" {
			return env
		}
		return "dev" // default to devnet
	}()
	ConfigFile string
	AppConfig  Config
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
		zstorConf:  filepath.Join(prefix, "etc", "zstor-default.toml"),
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
	namespaces, err := ioutil.ReadDir(h.zstorData)
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
		indexFiles, err := ioutil.ReadDir(indexDir)
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
	tmpDir, err := ioutil.TempDir("/tmp", "zdb.hook.tmp.XXXXXXXX")
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
	data, err := ioutil.ReadFile(src)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(dst, data, 0644)
}
