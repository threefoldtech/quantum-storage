package util

import (
	"fmt"
	"os"
	"path/filepath"
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
// These files might not exist on disk, since zstor can remove uploaded files,
// so it's a theoretical set
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

			// Add zdb-namespace file to result (always included when in index dir)
			if dir == "index" {
				result = append(result, filepath.Join(nsPath, "zdb-namespace"))
			}

			// Find the highest index number
			highestNum := -1
			for _, file := range files {
				if file.IsDir() {
					continue
				}
				name := file.Name()
				if name != "zdb-namespace" {
					if num, err := extractNumber(name); err == nil && num > highestNum {
						highestNum = num
					}
				}
			}

			// Generate all possible paths from 0 to highestNum-1
			if highestNum > 0 {
				for num := range highestNum {
					var path string
					if dir == "data" {
						// Generate 'd' prefixed paths for data directory
						path = filepath.Join(nsPath, fmt.Sprintf("d%d", num))
					} else if dir == "index" {
						// Generate 'i' prefixed paths for index directory
						path = filepath.Join(nsPath, fmt.Sprintf("i%d", num))
					}
					result = append(result, path)
				}
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
