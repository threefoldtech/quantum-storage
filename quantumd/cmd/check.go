package cmd

import (
	"bytes"
	"encoding/hex"
	"fmt"
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

		// Get all eligible files for upload
		eligibleFiles, err := util.GetEligibleZdbFiles(cfg.ZdbRootPath)
		if err != nil {
			return fmt.Errorf("failed to get eligible files: %w", err)
		}

		// Get all metadata from zstor once
		zstorClient, err := zstor.NewClient(cfg.ZstorConfigPath)
		if err != nil {
			return fmt.Errorf("failed to create zstor client: %w", err)
		}

		allMetadata, err := zstorClient.GetAllMetadata()
		if err != nil {
			return fmt.Errorf("failed to get metadata: %w", err)
		}

		// Assign filenames to metadata once
		filenameMetadata, err := zstor.AssignFilenamesToMetadata(eligibleFiles, allMetadata, cfg.ZdbRootPath)
		if err != nil {
			return fmt.Errorf("failed to assign filenames to metadata: %w", err)
		}

		if err := checkAndPrintHashes(eligibleFiles, filenameMetadata, cfg.ZdbRootPath); err != nil {
			return fmt.Errorf("error during hash check: %w", err)
		}

		if err := checkPendingUploads(eligibleFiles, filenameMetadata, cfg.ZdbRootPath); err != nil {
			return fmt.Errorf("error during pending upload check: %w", err)
		}

		return nil
	},
}

func checkAndPrintHashes(eligibleFiles []string, filenameMetadata map[string]zstor.Metadata, zdbRootPath string) error {
	var (
		mismatches int
		files      int
		notFound   int
	)

	fmt.Printf("%-70s %-35s %-35s %-10s\n", "File Path", "Remote Hash", "Local Hash", "Status")
	fmt.Println(strings.Repeat("-", 150))

	// Process each file from metadata with actual filenames
	for _, file := range eligibleFiles {
		files++

		var status string
		var localHash []byte
		var remoteHash []byte
		var localHashDisplay string
		var remoteHashDisplay string

		metadata, existsRemote := filenameMetadata[file]
		if existsRemote {
			// File exists in metadata, get its remote hash
			remoteHash = metadata.Checksum
			remoteHashDisplay = hex.EncodeToString(remoteHash)
		} else {
			// File does not exist in metadata, remote hash is nil
			remoteHash = nil
			remoteHashDisplay = "N/A"
		}

		if _, err := os.Stat(file); os.IsNotExist(err) {
			localHash = nil
			localHashDisplay = "N/A"
		} else {
			localHash = zstor.GetLocalHash(file)
			localHashDisplay = hex.EncodeToString(localHash)
		}

		// Check if local file exists
		if existsRemote && remoteHash != nil && localHash == nil {
			// File is in metadata but not on local disk - this is okay
			status = "OK (Remote)"
			notFound++
		} else if localHash != nil && remoteHash == nil {
			// File exists locally but not in metadata - pending upload
			status = "Pending"
		} else if remoteHash != nil && localHash != nil {
			// File exists locally, compare hashes
			if bytes.Equal(remoteHash, localHash) {
				status = "OK"
			} else {
				status = "Mismatch"
				mismatches++
			}
		}

		fmt.Printf("%-70s %-35s %-35s %-10s\n", file, remoteHashDisplay, localHashDisplay, status)
	}

	if files == 0 {
		fmt.Println("No eligible files found.")
	}

	fmt.Println()
	summary := fmt.Sprintf("Hash Check Summary: %d files checked. %d mismatches, %d files not found on disk.", files, mismatches, notFound)
	fmt.Println(summary)

	// Do not return an error for mismatches, just report them.
	// An error should only be returned for operational failures.
	return nil
}

func checkPendingUploads(eligibleFiles []string, filenameMetadata map[string]zstor.Metadata, zdbRootPath string) error {
	// Create a map for quick lookup of file paths from metadata
	uploadedFilePaths := make(map[string]bool)
	for filePath := range filenameMetadata {
		uploadedFilePaths[filePath] = true
	}

	var pendingUploads []string
	for _, file := range eligibleFiles {
		// Check if file exists in metadata (meaning it's uploaded)
		if !uploadedFilePaths[file] {
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
