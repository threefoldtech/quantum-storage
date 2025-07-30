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
	BinaryPath string
	ConfigPath string
}

// NewClient creates a new zstor client.
func NewClient(binaryPath, configPath string) (*Client, error) {
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("zstor binary not found at %s", binaryPath)
	}
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("zstor config not found at %s", configPath)
	}
	return &Client{
		BinaryPath: binaryPath,
		ConfigPath: configPath,
	}, nil
}

// Store uploads a file to zstor. It handles both data and index files.
// For index files, it can create a temporary snapshot to ensure atomicity.
func (c *Client) Store(filePath string, useSnapshot bool) error {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil // Don't treat non-existent files as an error
	}

	uploadPath := filePath
	var tmpDir string
	var err error

	if useSnapshot {
		// Create a temporary directory for the snapshot
		tmpDir, err = os.MkdirTemp("/tmp", "zstor-upload-")
		if err != nil {
			return fmt.Errorf("failed to create temp dir for index snapshot: %w", err)
		}
		defer os.RemoveAll(tmpDir)

		// Copy the index file to the temp directory
		tmpPath := filepath.Join(tmpDir, filepath.Base(filePath))
		if err := copyFile(filePath, tmpPath); err != nil {
			return fmt.Errorf("failed to copy index file to temp dir: %w", err)
		}
		uploadPath = tmpPath
	}

	args := []string{"-c", c.ConfigPath, "store", "-s", "--file", uploadPath}
	if useSnapshot {
		// When a key is provided, zstor uses the basename of the file being uploaded
		// and constructs the final remote path relative to the key directory.
		// For index files, we want the remote path to be the original file path.
		args = append(args, "-k", filepath.Dir(filePath))
	}

	cmd := exec.Command(c.BinaryPath, args...)
	log.Printf("Executing: %s", cmd.String())

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to store %s: %v. Output: %s", filePath, err, string(output))
	}

	log.Printf("Successfully stored: %s", filePath)
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

// copyFile is a utility function to copy file content.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}
