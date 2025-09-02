package util

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func MapIPs(ips []string) map[string]string {
	mapped := make(map[string]string)
	for _, ip := range ips {
		parts := strings.Split(ip, ":")
		if len(parts) == 0 {
			continue
		}
		firstPart := parts[0]
		// Convert the first part to a hex number for range checking
		hexValue, err := strconv.ParseInt(firstPart, 16, 64)
		if err != nil {
			continue
		}
		// Check if it falls into specific ranges
		if 0x2000 <= hexValue && hexValue <= 0x3FFF {
			mapped["ipv6"] = ip
		} else if 0x200 <= hexValue && hexValue <= 0x3FF {
			mapped["ygg"] = ip
		} else if 0x400 <= hexValue && hexValue <= 0x5FF {
			mapped["mycelium"] = ip
		}
	}
	return mapped
}

// GetEligibleZdbFiles returns all files that are eligible for upload into zstor
// It takes a base path for zdb data and returns full paths of eligible files
func GetEligibleZdbFiles(basePath string) ([]string, error) {
	var result []string

	// Define the directories to check
	dirs := []string{"data", "index"}

	// Define the namespaces to process (excluding zdbfs-temp)
	namespaces := []string{"zdbfs-data", "zdbfs-meta"}

	for _, dir := range dirs {
		dirPath := filepath.Join(basePath, dir)

		// Check if directory exists
		if _, err := os.Stat(dirPath); os.IsNotExist(err) {
			continue
		}

		for _, namespace := range namespaces {
			nsPath := filepath.Join(dirPath, namespace)

			// Check if namespace directory exists
			if _, err := os.Stat(nsPath); os.IsNotExist(err) {
				continue
			}

			// Get all files in the namespace directory
			files, err := os.ReadDir(nsPath)
			if err != nil {
				return nil, fmt.Errorf("error reading directory %s: %w", nsPath, err)
			}

			// Separate numbered files from zdb-namespace files
			var numberedFiles []string
			var zdbNamespaceFiles []string

			for _, file := range files {
				if file.IsDir() {
					continue
				}

				name := file.Name()
				if name == "zdb-namespace" {
					zdbNamespaceFiles = append(zdbNamespaceFiles, filepath.Join(nsPath, name))
				} else {
					numberedFiles = append(numberedFiles, name)
				}
			}

			// Sort numbered files to identify the highest number
			sort.Slice(numberedFiles, func(i, j int) bool {
				// Extract numbers from filenames for proper sorting
				numI, errI := extractNumber(numberedFiles[i])
				numJ, errJ := extractNumber(numberedFiles[j])

				// If conversion fails for either, we can't sort properly
				if errI != nil || errJ != nil {
					// In case of error, maintain original order
					return i < j
				}

				return numI < numJ
			})

			// Add zdb-namespace files to result (always included)
			result = append(result, zdbNamespaceFiles...)

			// Add numbered files except the highest one
			for i := 0; i < len(numberedFiles)-1; i++ { // -1 to exclude the last (highest) file
				result = append(result, filepath.Join(nsPath, numberedFiles[i]))
			}
		}
	}

	return result, nil
}

// extractNumber extracts the numeric part from a filename like "d0" or "i1"
func extractNumber(filename string) (int, error) {
	if len(filename) < 2 {
		return 0, fmt.Errorf("filename too short")
	}

	// Check if first character is 'd' or 'i'
	if filename[0] != 'd' && filename[0] != 'i' {
		return 0, fmt.Errorf("invalid filename prefix")
	}

	// Extract the number part
	numStr := filename[1:]
	return strconv.Atoi(numStr)
}
