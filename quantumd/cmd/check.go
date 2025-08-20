package cmd

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	_ "github.com/mattn/go-sqlite3"
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
	Long: `Queries the SQLite database to list all uploaded files, showing their
database hash versus their current local hash. It helps in verifying the integrity
of the stored files. It also checks for any pending uploads.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadConfig(ConfigFile)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		dbPath := cfg.DatabasePath
		if dbPath == "" {
			dbPath = filepath.Join(cfg.ZdbRootPath, "uploaded_files.db")
		}

		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			return fmt.Errorf("database file not found at %s", dbPath)
		}

		if err := checkAndPrintHashes(dbPath); err != nil {
			return fmt.Errorf("error during hash check: %w", err)
		}

		if err := checkPendingUploads(cfg.ZdbRootPath, dbPath); err != nil {
			return fmt.Errorf("error during pending upload check: %w", err)
		}

		return nil
	},
}

func checkAndPrintHashes(dbPath string) error {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	rows, err := db.Query("SELECT file_path, hash FROM uploaded_files ORDER BY file_path")
	if err != nil {
		return fmt.Errorf("failed to query uploaded files: %w", err)
	}
	defer rows.Close()

	var (
		filePath   string
		dbHash     string
		localHash  string
		status     string
		mismatches int
		files      int
		notFound   int
	)

	fmt.Printf("%-70s %-35s %-35s %-10s\n", "File Path", "Database Hash", "Local Hash", "Status")
	fmt.Println(string(make([]byte, 150, 150)))

	for rows.Next() {
		files++
		if err := rows.Scan(&filePath, &dbHash); err != nil {
			return fmt.Errorf("failed to scan row: %w", err)
		}

		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			localHash = "Not Found"
			status = "Missing"
			notFound++
		} else {
			localHash = zstor.GetLocalHash(filePath)
			if dbHash == localHash {
				status = "OK"
			} else {
				status = "Mismatch"
				mismatches++
			}
		}

		fmt.Printf("%-70s %-35s %-35s %-10s\n", filePath, dbHash, localHash, status)
	}

	if files == 0 {
		fmt.Println("No uploaded files found in the database.")
	}

	fmt.Println()
	summary := fmt.Sprintf("Hash Check Summary: %d files checked. %d mismatches, %d files not found.", files, mismatches, notFound)
	fmt.Println(summary)

	// Do not return an error for mismatches, just report them.
	// An error should only be returned for operational failures.
	return nil
}

func checkPendingUploads(zdbRootPath, dbPath string) error {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database for pending check: %w", err)
	}
	defer db.Close()

	isUploaded := func(filePath string) (bool, error) {
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM uploaded_files WHERE file_path = ?", filePath).Scan(&count)
		if err != nil {
			return false, err
		}
		return count > 0, nil
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
			uploaded, err := isUploaded(file)
			if err != nil {
				log.Printf("Failed to check upload status for %s: %v", file, err)
				continue
			}
			if !uploaded {
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
