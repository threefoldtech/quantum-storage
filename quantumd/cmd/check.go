package cmd

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"github.com/spf13/cobra"
)

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Check for missing but expected uploads in the database",
	Long: `Queries the SQLite database to see if all expected data and index files
have been uploaded. It checks for d$i and i$i files up to the maximum
index found in the database. If any files are missing, it prints a list of them.`,
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

		if cfg.ZdbRootPath == "" {
			return fmt.Errorf("zdb_root_path is not set in the configuration")
		}

		missingFiles, err := checkForMissingFiles(dbPath, cfg.ZdbRootPath)
		if err != nil {
			return fmt.Errorf("error during check: %w", err)
		}

		if len(missingFiles) == 0 {
			fmt.Println("All expected files are uploaded.")
		} else {
			fmt.Println("The following expected files are missing:")
			for _, file := range missingFiles {
				fmt.Println(file)
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(checkCmd)
}

func checkForMissingFiles(dbPath, zdbRootPath string) ([]string, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	rows, err := db.Query("SELECT file_path FROM uploaded_files")
	if err != nil {
		return nil, fmt.Errorf("failed to query uploaded files: %w", err)
	}
	defer rows.Close()

	uploadedMap := make(map[string]bool)
	maxIndex := -1
	namespaces := []string{"zdbfs-data", "zdbfs-meta"}

	for rows.Next() {
		var filePath string
		if err := rows.Scan(&filePath); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		uploadedMap[filePath] = true

		base := filepath.Base(filePath)
		if strings.HasPrefix(base, "d") || strings.HasPrefix(base, "i") {
			numStr := base[1:]
			num, err := strconv.Atoi(numStr)
			if err == nil && num > maxIndex {
				maxIndex = num
			}
		}
	}

	if maxIndex == -1 {
		return nil, nil // No indexed files found, nothing to check
	}

	var missingFiles []string
	for _, ns := range namespaces {
		dataDir := filepath.Join(zdbRootPath, "data", ns)
		indexDir := filepath.Join(zdbRootPath, "index", ns)

		for i := 0; i < maxIndex; i++ {
			// Check data files
			dFile := filepath.Join(dataDir, fmt.Sprintf("d%d", i))
			if _, found := uploadedMap[dFile]; !found {
				missingFiles = append(missingFiles, dFile)
			}

			// Check index files
			iFile := filepath.Join(indexDir, fmt.Sprintf("i%d", i))
			if _, found := uploadedMap[iFile]; !found {
				missingFiles = append(missingFiles, iFile)
			}
		}
	}

	sort.Strings(missingFiles)
	return missingFiles, nil
}


