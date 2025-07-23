package hook

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	// SocketPath is the path to the unix socket for hook communication
	SocketPath = "/tmp/zdb-hook.sock"
)

// UploadTracker defines the interface for tracking uploaded files.
// This helps in decoupling the hook handler from the concrete implementation.
type UploadTracker interface {
	MarkUploaded(filePath, hash string, fileSize int64) error
	IsUploaded(filePath string) (bool, error)
}

// Handler manages hook dispatching
type Handler struct {
	ZstorConf     string
	ZstorBin      string
	ZstorIndex    string
	ZstorData     string
	UploadTracker UploadTracker
}

// NewHandler creates a new hook handler
func NewHandler(zdbRootPath string, tracker UploadTracker) (*Handler, error) {
	h := &Handler{
		ZstorConf:     "/etc/zstor.toml",
		ZstorBin:      "/usr/local/bin/zstor",
		ZstorIndex:    filepath.Join(zdbRootPath, "index"),
		ZstorData:     filepath.Join(zdbRootPath, "data"),
		UploadTracker: tracker,
	}

	// Verify that the zstor binary exists
	if _, err := os.Stat(h.ZstorBin); os.IsNotExist(err) {
		return nil, fmt.Errorf("zstor binary not found at %s", h.ZstorBin)
	}

	return h, nil
}

// ListenAndServe starts the hook listener and serves hook requests
func (h *Handler) ListenAndServe() {
	// Ensure the socket doesn't already exist
	if err := os.RemoveAll(SocketPath); err != nil {
		log.Fatalf("Failed to remove existing socket: %v", err)
	}

	listener, err := net.Listen("unix", SocketPath)
	if err != nil {
		log.Fatalf("Failed to listen on unix socket %s: %v", SocketPath, err)
	}
	defer listener.Close()

	log.Printf("Daemon listening for hooks on %s", SocketPath)

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

func (h *Handler) handleConnection(conn net.Conn) {
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	if scanner.Scan() {
		line := scanner.Text()
		log.Printf("Received hook message: %s", line)

		parts := strings.Fields(line)
		if len(parts) == 0 {
			log.Println("Received empty hook message, ignoring.")
			fmt.Fprintf(conn, "ERROR: empty hook message\n")
			return
		}

		action := parts[0]
		args := parts[1:]

		// Check if this is a blocking hook
		isBlocking := action == "missing-data"

		if isBlocking {
			// Handle blocking hooks synchronously
			err := h.dispatchHook(action, args)
			if err != nil {
				log.Printf("Error handling blocking hook action '%s': %v", action, err)
				fmt.Fprintf(conn, "ERROR: %v\n", err)
			} else {
				fmt.Fprintf(conn, "SUCCESS: %s completed\n", action)
			}
		} else {
			// Handle non-blocking hooks asynchronously
			go func() {
				if err := h.dispatchHook(action, args); err != nil {
					log.Printf("Error handling hook action '%s': %v", action, err)
				}
			}()
			fmt.Fprintf(conn, "SUCCESS: %s queued\n", action)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Error reading from hook connection: %v", err)
		fmt.Fprintf(conn, "ERROR: %v\n", err)
	}
}

func (h *Handler) dispatchHook(action string, args []string) error {
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
		log.Printf("Ignoring unknown hook action: %s", action)
		return nil
	}
}

// uploadAndTrack handles uploading a file and marking it in the database.
func (h *Handler) uploadAndTrack(filePath string) {
	// Skip if file doesn't exist
	fileInfo, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return
	}

	// For data files, check if it's already uploaded to avoid re-uploading.
	isDataFile := strings.Contains(filePath, "/data/")
	if isDataFile {
		uploaded, err := h.UploadTracker.IsUploaded(filePath)
		if err != nil {
			log.Printf("Failed to check upload status for %s: %v", filePath, err)
		}
		if uploaded {
			log.Printf("Skipping already uploaded data file: %s", filePath)
			return
		}
	}

	// Use a single attempt version of runZstor
	args := []string{"-c", h.ZstorConf, "store", "-s", "--file", filePath}
	cmd := exec.Command(h.ZstorBin, args...)
	log.Printf("Executing: %s", cmd.String())

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Failed to upload %s: %v. Output: %s", filePath, err, string(output))
		return
	}

	log.Printf("Successfully uploaded: %s", filePath)

	// Track uploaded data files
	if isDataFile {
		hash := GetLocalHash(filePath)
		if hash == "" {
			log.Printf("Failed to get local hash for %s, cannot mark as uploaded", filePath)
			return
		}
		if err := h.UploadTracker.MarkUploaded(filePath, hash, fileInfo.Size()); err != nil {
			log.Printf("Failed to mark file as uploaded: %v", err)
		}
	}
}

// runZstor executes the zstor command with the given arguments
func (h *Handler) runZstor(args ...string) error {
	cmd := exec.Command(h.ZstorBin, args...)
	log.Printf("Executing: %s", cmd.String())

	output, err := cmd.CombinedOutput()
	if err == nil {
		log.Printf("Successfully executed: %s", cmd.String())
		return nil
	}

	log.Printf("Command failed: %s. Error: %v. Output: %s", cmd.String(), err, string(output))
	return err
}

func (h *Handler) handleClose() error {
	namespaces, err := os.ReadDir(h.ZstorData)
	if err != nil {
		return fmt.Errorf("could not read zstor data dir %s: %w", h.ZstorData, err)
	}

	for _, ns := range namespaces {
		nsName := ns.Name()
		if !ns.IsDir() || nsName == "zdbfs-temp" {
			continue
		}

		log.Printf("Processing 'close' for namespace: %s", nsName)
		indexDir := filepath.Join(h.ZstorIndex, nsName)
		dataDir := filepath.Join(h.ZstorData, nsName)

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
		go h.uploadAndTrack(dataFile)
		// Upload index file
		go h.uploadAndTrack(indexFile)
	}
	return nil
}

func (h *Handler) handleReady() error {
	// The script runs this in a loop, so we do the same.
	// This runs in the foreground of the hook handler, blocking this hook until ready.
	return h.runZstor("-c", h.ZstorConf, "test")
}

func (h *Handler) handleNamespaceUpdate(namespace string) error {
	if namespace == "zdbfs-temp" {
		log.Println("Skipping temporary namespace zdbfs-temp")
		return nil
	}
	file := filepath.Join(h.ZstorIndex, namespace, "zdb-namespace")
	// Run in a goroutine to not block the hook call
	go h.uploadAndTrack(file)
	return nil
}

func (h *Handler) handleJumpIndex(indexPath string, dirtyIndices []string) error {
	namespace := filepath.Base(filepath.Dir(indexPath))
	if namespace == "zdbfs-temp" {
		log.Println("Skipping temporary namespace zdbfs-temp")
		return nil
	}

	dirBase := filepath.Dir(indexPath)

	// Upload dirty index files
	for _, dirty := range dirtyIndices {
		fileName := fmt.Sprintf("i%s", dirty)
		src := filepath.Join(dirBase, fileName)
		go h.uploadAndTrack(src)
	}

	// Upload the main index file that triggered the jump
	go h.uploadAndTrack(indexPath)

	return nil
}

func (h *Handler) handleJumpData(dataPath string) error {
	namespace := filepath.Base(filepath.Dir(dataPath))
	if namespace == "zdbfs-temp" {
		log.Println("Skipping temporary namespace zdbfs-temp")
		return nil
	}
	// Run in a goroutine to not block the hook call
	go h.uploadAndTrack(dataPath)
	return nil
}

func (h *Handler) handleMissingData(dataPath string) error {
	// This needs to be synchronous, as zdb is waiting for the file.
	return h.runZstor("-c", h.ZstorConf, "retrieve", "--file", dataPath)
}

// copyFile utility
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

func GetLocalHash(file string) string {
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
