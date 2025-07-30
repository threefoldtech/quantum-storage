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

	"github.com/threefoldtech/quantum-storage/quantumd/internal/zstor"
)

const (
	// SocketPath is the path to the unix socket for hook communication
	SocketPath = "/tmp/zdb-hook.sock"
)

// UploadTracker defines the interface for tracking uploaded files.
type UploadTracker interface {
	MarkUploaded(filePath, hash string, fileSize int64) error
	IsUploaded(filePath string) (bool, error)
}


// Handler manages hook dispatching
type Handler struct {
	ZstorIndex    string
	ZstorData     string
	UploadTracker UploadTracker
	Zstor         *zstor.Client
}

// NewHandler creates a new hook handler
func NewHandler(zdbRootPath string, tracker UploadTracker, zstorClient *zstor.Client) (*Handler, error) {
	h := &Handler{
		ZstorIndex:    filepath.Join(zdbRootPath, "index"),
		ZstorData:     filepath.Join(zdbRootPath, "data"),
		UploadTracker: tracker,
		Zstor:         zstorClient,
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
		isBlocking := action == "missing-data" || action == "ready"

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
			// For non-blocking hooks, we can respond immediately.
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
		if len(args) < 2 {
			return fmt.Errorf("not enough arguments for %s", action)
		}
		var dirtyList []string
		if len(args) >= 4 {
			dirtyList = args[3:]
		}
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
func (h *Handler) uploadAndTrack(filePath string, isIndex bool) {
	fileInfo, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return
	}

	isDataFile := !isIndex

	if isDataFile {
		uploaded, err := h.UploadTracker.IsUploaded(filePath)
		if err != nil {
			log.Printf("Failed to check upload status for %s: %v", filePath, err)
			// Continue anyway, better to re-upload than to miss an upload.
		}
		if uploaded {
			log.Printf("Skipping already uploaded data file: %s", filePath)
			return
		}
	}

	// For hooks, we always use a snapshot for index files to ensure atomicity.
	if err := h.Zstor.Store(filePath, isIndex); err != nil {
		log.Printf("Failed to upload %s: %v", filePath, err)
		return
	}

	hash := zstor.GetLocalHash(filePath)
	if hash == "" {
		log.Printf("Failed to get local hash for %s, cannot mark as uploaded", filePath)
		return
	}
	if err := h.UploadTracker.MarkUploaded(filePath, hash, fileInfo.Size()); err != nil {
		log.Printf("Failed to mark file as uploaded: %v", err)
	}
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

		lastActive, err := findLastActiveFile(indexDir)
		if err != nil {
			log.Printf("Could not find active files for namespace %s: %v. Skipping.", nsName, err)
			continue
		}

		dataFile := filepath.Join(dataDir, fmt.Sprintf("d%d", lastActive))
		indexFile := filepath.Join(indexDir, fmt.Sprintf("i%d", lastActive))

		go h.uploadAndTrack(dataFile, false)
		go h.uploadAndTrack(indexFile, true)
	}
	return nil
}

func (h *Handler) handleReady() error {
	return h.Zstor.Test()
}

func (h *Handler) handleNamespaceUpdate(namespace string) error {
	if namespace == "zdbfs-temp" {
		log.Println("Skipping temporary namespace zdbfs-temp")
		return nil
	}
	file := filepath.Join(h.ZstorIndex, namespace, "zdb-namespace")
	go h.uploadAndTrack(file, true)
	return nil
}

func (h *Handler) handleJumpIndex(indexPath string, dirtyIndices []string) error {
	namespace := filepath.Base(filepath.Dir(indexPath))
	if namespace == "zdbfs-temp" {
		log.Println("Skipping temporary namespace zdbfs-temp")
		return nil
	}

	dirBase := filepath.Dir(indexPath)

	for _, dirty := range dirtyIndices {
		fileName := fmt.Sprintf("i%s", dirty)
		src := filepath.Join(dirBase, fileName)
		go h.uploadAndTrack(src, true)
	}

	indexNum := strings.TrimPrefix(filepath.Base(indexPath), "i")
	isAlreadyDirty := false
	for _, dirty := range dirtyIndices {
		if dirty == indexNum {
			isAlreadyDirty = true
			break
		}
	}

	if !isAlreadyDirty {
		go h.uploadAndTrack(indexPath, true)
	}

	return nil
}

func (h *Handler) handleJumpData(dataPath string) error {
	namespace := filepath.Base(filepath.Dir(dataPath))
	if namespace == "zdbfs-temp" {
		log.Println("Skipping temporary namespace zdbfs-temp")
		return nil
	}
	go h.uploadAndTrack(dataPath, false)
	return nil
}

func (h *Handler) handleMissingData(dataPath string) error {
	return h.Zstor.Retrieve(dataPath)
}

func findLastActiveFile(dir string) (int, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return -1, err
	}

	lastActive := -1
	for _, f := range files {
		if strings.HasPrefix(f.Name(), "i") {
			numPart := strings.TrimPrefix(f.Name(), "i")
			num, err := strconv.Atoi(numPart)
			if err == nil && num > lastActive {
				lastActive = num
			}
		}
	}

	if lastActive == -1 {
		return -1, fmt.Errorf("no active index files found")
	}
	return lastActive, nil
}

// SetupSymlink ensures the hook symlink is in place
func SetupSymlink() error {
	src, err := exec.LookPath("quantumd")
	if err != nil {
		return fmt.Errorf("could not find quantumd executable in PATH: %w", err)
	}

	dest := "/usr/local/bin/quantumd-hook"

	if fi, err := os.Lstat(dest); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(dest)
			if err == nil && link == src {
				log.Printf("Symlink already exists and is correct: %s -> %s", dest, src)
				return nil
			}
		}
		if err := os.Remove(dest); err != nil {
			return fmt.Errorf("failed to remove existing file at %s: %w", dest, err)
		}
	}

	log.Printf("Creating symlink from %s to %s", src, dest)
	return os.Symlink(src, dest)
}
