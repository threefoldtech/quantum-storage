package cmd

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/mattn/go-sqlite3"
	"github.com/spf13/cobra"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/hook"
)

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Check for missing files and list uploaded files with their hashes.",
	Long: `Queries the SQLite database to list all uploaded files, showing their
database hash versus their current local hash. It helps in verifying the integrity
of the stored files.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadConfig(ConfigFile)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		dbPath := cfg.DatabasePath
		if dbPath == "" {
			// Fallback to default location if not specified
			dbPath = "/data/uploaded_files.db"
		}

		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			return fmt.Errorf("database file not found at %s", dbPath)
		}

		if err := checkAndPrintHashes(dbPath); err != nil {
			return fmt.Errorf("error during check: %w", err)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(checkCmd)
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
		filePath    string
		dbHash      string
		localHash   string
		status      string
		mismatches  int
		files       int
		notFound    int
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
			localHash = hook.GetLocalHash(filePath)
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
		return nil
	}

	fmt.Println()
	summary := fmt.Sprintf("Summary: %d files checked. %d mismatches, %d files not found.", files, mismatches, notFound)
	fmt.Println(summary)

	if mismatches > 0 || notFound > 0 {
		return fmt.Errorf("integrity check failed with %d mismatches and %d missing files", mismatches, notFound)
	}

	return nil
}
