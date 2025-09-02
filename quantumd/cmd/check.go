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

		// Assign filenames to metadata once
		filenameMetadata, err := zstor.AssignFilenamesToMetadata(allMetadata, cfg.ZdbRootPath)
		if err != nil {
			return fmt.Errorf("failed to assign filenames to metadata: %w", err)
		}

		if err := checkAndPrintHashes(filenameMetadata, cfg.ZdbRootPath); err != nil {
			return fmt.Errorf("error during hash check: %w", err)
		}

		if err := checkPendingUploads(filenameMetadata, cfg.ZdbRootPath); err != nil {
			return fmt.Errorf("error during pending upload check: %w", err)
		}

		return nil
	},
}

func checkAndPrintHashes(filenameMetadata map[string]zstor.Metadata, zdbRootPath string) error {
	var (
		status     string
		mismatches int
		files      int
		notFound   int
	)

	fmt.Printf("%-70s %-35s %-35s %-10s\n", "File Path", "Remote Hash", "Local Hash", "Status")
	fmt.Println(strings.Repeat("-", 150))

	// Process each file from metadata with actual filenames
	for localPath, metadata := range filenameMetadata {
		files++

		// Convert remote hash to hex string for comparison
		dbHash := metadata.Checksum

		// Check if local file exists
		var actualLocalPath string
		if _, err := os.Stat(localPath); os.IsNotExist(err) {
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
		if status != "Missing" {
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

func checkPendingUploads(filenameMetadata map[string]zstor.Metadata, zdbRootPath string) error {
	// Create a map for quick lookup of file paths from metadata
	uploadedFilePaths := make(map[string]bool)
	for filePath := range filenameMetadata {
		uploadedFilePaths[filePath] = true
	}

	// Get all eligible files for upload
	eligibleFiles, err := util.GetEligibleZdbFiles(zdbRootPath)
	if err != nil {
		return fmt.Errorf("failed to get eligible files: %w", err)
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
