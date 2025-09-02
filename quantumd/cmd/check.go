package cmd

import (
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/config"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/util"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/zstor"
)

func init() {
	rootCmd.AddCommand(checkCmd)
}

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Check for missing files and list uploaded files with their hashes.",
	Long: `Queries zstor metadata to list all uploaded files, showing their
remote hash versus their current local hash. It helps in verifying the integrity
of the stored files. It also checks for any pending uploads.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadConfig(ConfigFile)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		// Get all metadata from zstor once
		allMetadata, err := zstor.GetAllMetadata(cfg.ZstorConfigPath)
		if err != nil {
			return fmt.Errorf("failed to get metadata: %w", err)
		}

		if err := checkAndPrintHashes(allMetadata, cfg.ZdbRootPath); err != nil {
			return fmt.Errorf("error during hash check: %w", err)
		}

		if err := checkPendingUploads(cfg.ZdbRootPath, allMetadata); err != nil {
			return fmt.Errorf("error during pending upload check: %w", err)
		}

		return nil
	},
}

func checkAndPrintHashes(allMetadata map[string]zstor.Metadata, zdbRootPath string) error {
	var (
		status     string
		mismatches int
		files      int
		notFound   int
	)

	fmt.Printf("%-70s %-35s %-35s %-10s\n", "File Path", "Remote Hash", "Local Hash", "Status")
	fmt.Println(string(make([]byte, 150, 150)))

	// Create a map of local file hashes to their actual paths
	localFiles := make(map[string]string)

	// Get all eligible files for upload
	eligibleFiles, err := util.GetEligibleZdbFiles(zdbRootPath)
	if err != nil {
		return fmt.Errorf("failed to get eligible files: %w", err)
	}

	// Hash the paths of eligible files
	for _, path := range eligibleFiles {
		// Hash the file path using the same method as zstor
		hashedPath := zstor.GetPathHash(path)
		localFiles[hashedPath] = path
	}

	// Process each file from metadata
	for zstorPath, metadata := range allMetadata {
		files++

		// Extract the hash part from the zstor path (/zstor-meta/meta/{hash})
		parts := strings.Split(zstorPath, "/")
		if len(parts) < 3 {
			log.Printf("Unexpected zstor path format: %s", zstorPath)
			continue
		}
		fileHash := parts[len(parts)-1]

		// Find the corresponding local file path
		localPath, exists := localFiles[fileHash]
		if !exists {
			// Try to find if there's a local file with this exact path
			if _, err := os.Stat(zstorPath); err == nil {
				localPath = zstorPath
			} else {
				localPath = "Unknown (hash: " + fileHash + ")"
			}
		}

		// Convert remote hash to hex string for comparison
		dbHash := metadata.Checksum

		// Check if local file exists
		var actualLocalPath string
		if strings.HasPrefix(localPath, "Unknown") {
			status = "Missing"
			notFound++
			actualLocalPath = localPath
		} else if _, err := os.Stat(localPath); os.IsNotExist(err) {
			status = "Missing"
			notFound++
			actualLocalPath = localPath
		} else {
			actualLocalPath = localPath
			// Calculate local hash of the actual file content
			localHashStr := zstor.GetLocalHash(localPath)
			localHash, err := hex.DecodeString(localHashStr)
			if err != nil {
				log.Printf("Failed to decode local hash for %s: %v", localPath, err)
				status = "Error"
			} else if string(dbHash) == string(localHash) {
				status = "OK"
			} else {
				status = "Mismatch"
				mismatches++
			}
		}

		dbHashStr := hex.EncodeToString([]byte(dbHash))
		localHashDisplay := "Not Found"
		if status != "Missing" && !strings.HasPrefix(actualLocalPath, "Unknown") {
			localHashDisplay = zstor.GetLocalHash(actualLocalPath)
		}

		fmt.Printf("%-70s %-35s %-35s %-10s\n", actualLocalPath, dbHashStr, localHashDisplay, status)
	}

	if files == 0 {
		fmt.Println("No uploaded files found in the metadata.")
	}

	fmt.Println()
	summary := fmt.Sprintf("Hash Check Summary: %d files checked. %d mismatches, %d files not found.", files, mismatches, notFound)
	fmt.Println(summary)

	// Do not return an error for mismatches, just report them.
	// An error should only be returned for operational failures.
	return nil
}

func checkPendingUploads(zdbRootPath string, allMetadata map[string]zstor.Metadata) error {
	// Create a map for quick lookup of file hashes from metadata
	uploadedFileHashes := make(map[string]bool)
	for zstorPath := range allMetadata {
		// Extract the hash part from the zstor path (/zstor-meta/meta/{hash})
		parts := strings.Split(zstorPath, "/")
		if len(parts) >= 3 {
			fileHash := parts[len(parts)-1]
			uploadedFileHashes[fileHash] = true
		}
	}

	// Get all eligible files for upload
	eligibleFiles, err := util.GetEligibleZdbFiles(zdbRootPath)
	if err != nil {
		return fmt.Errorf("failed to get eligible files: %w", err)
	}

	var pendingUploads []string
	for _, file := range eligibleFiles {
		// Hash the local file path to check against metadata
		fileHash := zstor.GetLocalHash(file)
		// Check if file exists in metadata (meaning it's uploaded)
		if !uploadedFileHashes[fileHash] {
			pendingUploads = append(pendingUploads, file)
		}
	}

	fmt.Println()
	if len(pendingUploads) > 0 {
		fmt.Println("Pending Uploads Report:")
		for _, file := range pendingUploads {
			fmt.Printf(" - %s\n", file)
		}
		summary := fmt.Sprintf("Upload Status Summary: %d files are pending upload.", len(pendingUploads))
		fmt.Println(summary)
	} else {
		fmt.Println("Upload Status Summary: All uploads are completed.")
	}

	return nil
}
