package cmd

import (
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/config"
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

		if err := checkAndPrintHashes(allMetadata); err != nil {
			return fmt.Errorf("error during hash check: %w", err)
		}

		if err := checkPendingUploads(cfg.ZdbRootPath, allMetadata); err != nil {
			return fmt.Errorf("error during pending upload check: %w", err)
		}

		return nil
	},
}

func checkAndPrintHashes(allMetadata map[string]zstor.Metadata) error {
	var (
		status     string
		mismatches int
		files      int
		notFound   int
	)

	fmt.Printf("%-70s %-35s %-35s %-10s\n", "File Path", "Remote Hash", "Local Hash", "Status")
	fmt.Println(string(make([]byte, 150, 150)))

	// Process each file from metadata
	for filePath, metadata := range allMetadata {
		files++
		
		// Convert remote hash to hex string for comparison
		dbHash := metadata.Checksum
		
		// Check if local file exists
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			status = "Missing"
			notFound++
			
			dbHashStr := hex.EncodeToString([]byte(dbHash))
			fmt.Printf("%-70s %-35s %-35s %-10s\n", filePath, dbHashStr, "Not Found", status)
		} else {
			// Calculate local hash
			localHashStr := zstor.GetLocalHash(filePath)
			localHash, err := hex.DecodeString(localHashStr)
			if err != nil {
				log.Printf("Failed to decode local hash for %s: %v", filePath, err)
				status = "Error"
				
				dbHashStr := hex.EncodeToString([]byte(dbHash))
				fmt.Printf("%-70s %-35s %-35s %-10s\n", filePath, dbHashStr, "Error", status)
			} else if string(dbHash) == string(localHash) {
				status = "OK"
				
				dbHashStr := hex.EncodeToString([]byte(dbHash))
				fmt.Printf("%-70s %-35s %-35s %-10s\n", filePath, dbHashStr, localHashStr, status)
			} else {
				status = "Mismatch"
				mismatches++
				
				dbHashStr := hex.EncodeToString([]byte(dbHash))
				fmt.Printf("%-70s %-35s %-35s %-10s\n", filePath, dbHashStr, localHashStr, status)
			}
		}
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
	// Create a map for quick lookup
	uploadedFiles := make(map[string]bool)
	for filePath := range allMetadata {
		uploadedFiles[filePath] = true
	}

	var pendingUploads []string
	zstorDataPath := filepath.Join(zdbRootPath, "data")

	namespaces, err := os.ReadDir(zstorDataPath)
	if err != nil {
		return fmt.Errorf("failed to read zstor data dir: %w", err)
	}

	for _, ns := range namespaces {
		if !ns.IsDir() || ns.Name() == "zdbfs-temp" {
			continue
		}

		nsPath := filepath.Join(zstorDataPath, ns.Name())
		files, err := filepath.Glob(filepath.Join(nsPath, "d*"))
		if err != nil {
			log.Printf("Failed to list data files in %s: %v", nsPath, err)
			continue
		}

		if len(files) <= 1 {
			continue
		}

		sort.Slice(files, func(i, j int) bool {
			numI, _ := strconv.Atoi(filepath.Base(files[i])[1:])
			numJ, _ := strconv.Atoi(filepath.Base(files[j])[1:])
			return numI < numJ
		})

		// All files except the last one (highest number) should have been uploaded.
		filesToCheck := files[:len(files)-1]
		for _, file := range filesToCheck {
			// Check if file exists in metadata (meaning it's uploaded)
			if !uploadedFiles[file] {
				pendingUploads = append(pendingUploads, file)
			}
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
