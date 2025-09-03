package zstor

import (
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/blake2b"
)

// Client provides a high-level API for interacting with the zstor binary.
type Client struct {
	BinaryPath          string
	ConfigPath          string
	MetadataDecoderPath string
}

// NewClient creates a new zstor client.
func NewClient(configPath string) (*Client, error) {
	zstorPath, err := exec.LookPath("zstor")
	if err != nil {
		return nil, fmt.Errorf("zstor binary not found in PATH: %w", err)
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("zstor config not found at %s", configPath)
	}

	decoderPath, err := exec.LookPath("zstor-metadata-decoder")
	if err != nil {
		return nil, fmt.Errorf("zstor-metadata-decoder binary not found in PATH: %w", err)
	}

	return &Client{
		BinaryPath:          zstorPath,
		ConfigPath:          configPath,
		MetadataDecoderPath: decoderPath,
	}, nil
}

// Store uploads a single file to zstor. This is primarily for data files.
// Index files should use StoreBatch.
func (c *Client) Store(filePath string) error {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil // Don't treat non-existent files as an error
	}

	args := []string{"-c", c.ConfigPath, "store", "-s", "--file", filePath}

	cmd := exec.Command(c.BinaryPath, args...)
	log.Printf("Executing: %s", cmd.String())

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to store %s: %v. Output: %s", filePath, err, string(output))
	}

	log.Printf("Successfully stored: %s", filePath)
	return nil
}

// StoreBatch uploads a batch of files from a temporary directory.
func (c *Client) StoreBatch(files []string, originalDir string) error {
	if len(files) == 0 {
		return nil
	}

	tmpDir, err := os.MkdirTemp("/tmp", "zstor-batch-upload-")
	if err != nil {
		return fmt.Errorf("failed to create temp dir for batch upload: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	for _, file := range files {
		if _, err := os.Stat(file); os.IsNotExist(err) {
			continue // Skip non-existent files
		}
		tmpPath := filepath.Join(tmpDir, filepath.Base(file))
		if err := copyFile(file, tmpPath); err != nil {
			return fmt.Errorf("failed to copy file %s to temp dir: %w", file, err)
		}
	}

	// -d for directory mode, -f for file (which is actually the directory path here)
	args := []string{"-c", c.ConfigPath, "store", "-s", "-d", "-f", tmpDir, "-k", originalDir}

	cmd := exec.Command(c.BinaryPath, args...)
	log.Printf("Executing batch store: %s", cmd.String())

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to store batch from %s: %v. Output: %s", tmpDir, err, string(output))
	}

	log.Printf("Successfully stored batch from: %s", tmpDir)
	return nil
}

// Check retrieves the remote hash of a file.
func (c *Client) Check(filePath string) (string, error) {
	cmd := exec.Command(c.BinaryPath, "-c", c.ConfigPath, "check", "--file", filePath)
	output, err := cmd.Output()
	if err != nil {
		// zstor check returns non-zero exit code if file not found, which is not an error here.
		return "", nil
	}
	return strings.TrimSpace(string(output)), nil
}

// Retrieve downloads a file from zstor.
func (c *Client) Retrieve(filePath string) error {
	cmd := exec.Command(c.BinaryPath, "-c", c.ConfigPath, "retrieve", "--file", filePath)
	log.Printf("Executing: %s", cmd.String())

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to retrieve %s: %v. Output: %s", filePath, err, string(output))
	}

	log.Printf("Successfully retrieved: %s", filePath)
	return nil
}

// Test checks the connection to the zstor backend.
func (c *Client) Test() error {
	cmd := exec.Command(c.BinaryPath, "-c", c.ConfigPath, "test")
	log.Printf("Executing: %s", cmd.String())

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("zstor test failed: %v. Output: %s", err, string(output))
	}
	return nil
}

// GetLocalHash computes the BLAKE2b-128 hash of a file.
func GetLocalHash(file string) string {
	f, err := os.Open(file)
	if err != nil {
		log.Printf("failed to open file for hashing %s: %v", file, err)
		return ""
	}
	defer f.Close()

	// b2sum -l 128 is BLAKE2b with a 128-bit (16-byte) digest.
	// The key parameter is nil because we are not using a keyed hash.
	h, err := blake2b.New(16, nil)
	if err != nil {
		log.Printf("failed to create blake2b hash: %v", err)
		return ""
	}

	if _, err := io.Copy(h, f); err != nil {
		log.Printf("failed to hash file %s: %v", file, err)
		return ""
	}

	return hex.EncodeToString(h.Sum(nil))
}

// GetPathHash computes the BLAKE2b-128 hash of a path. These are used by zstor
// when storing metadata, whereas the paths themselves are not stored
func GetPathHash(path string) string {
	h, err := blake2b.New(16, nil)
	if err != nil {
		log.Printf("failed to create blake2b hash: %v", err)
		return ""
	}

	_, err = h.Write([]byte(path))
	if err != nil {
		log.Printf("failed to hash path %s: %v", path, err)
		return ""
	}

	return hex.EncodeToString(h.Sum(nil))
}

// copyFile is a utility function to copy file content.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}
