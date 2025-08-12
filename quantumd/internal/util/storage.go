package util

import (
	"fmt"
	"strconv"
	"strings"
)

func ComputeBackendSize(totalDesiredBytes, expectedShards, minimalShards int64) (int64, error) {
	if totalDesiredBytes <= 0 {
		return 0, fmt.Errorf("totalDesiredBytes must be > 0")
	}
	if expectedShards <= 0 {
		return 0, fmt.Errorf("expectedShards must be > 0")
	}
	if minimalShards <= 0 {
		return 0, fmt.Errorf("minimalShards must be > 0")
	}
	if minimalShards > expectedShards {
		return 0, fmt.Errorf("minimalShards cannot exceed expectedShards")
	}

	overheadMultiplier := expectedShards / minimalShards
	perBackend := (totalDesiredBytes / expectedShards) * overheadMultiplier
	return perBackend, nil
}

func ComputeTotalStorage(backendSizeBytes, expectedShards, minimalShards int64) (int64, error) {
	if backendSizeBytes <= 0 {
		return 0, fmt.Errorf("backendSizeBytes must be > 0")
	}
	if expectedShards <= 0 {
		return 0, fmt.Errorf("expectedShards must be > 0")
	}
	if minimalShards <= 0 {
		return 0, fmt.Errorf("minimalShards must be > 0")
	}
	if minimalShards > expectedShards {
		return 0, fmt.Errorf("minimalShards cannot exceed expectedShards")
	}

	// To reverse the calculation, we need to account for the overhead multiplier.
	// The formula for backend size is:
	// backend = (total / expected) * (expected / minimal)
	// backend = total / minimal
	// So, total = backend * minimal
	return backendSizeBytes * minimalShards, nil
}

func ParseSize(sizeStr string) (uint64, error) {
	sizeStr = strings.ToUpper(strings.TrimSpace(sizeStr))
	if sizeStr == "" {
		return 0, nil
	}

	var multiplier uint64
	var unit string

	if strings.HasSuffix(sizeStr, "T") || strings.HasSuffix(sizeStr, "TB") {
		multiplier = 1024 * 1024 * 1024 * 1024
		unit = "TB"
		if strings.HasSuffix(sizeStr, "T") {
			unit = "T"
		}
	} else if strings.HasSuffix(sizeStr, "G") || strings.HasSuffix(sizeStr, "GB") {
		multiplier = 1024 * 1024 * 1024
		unit = "GB"
		if strings.HasSuffix(sizeStr, "G") {
			unit = "G"
		}
	} else if strings.HasSuffix(sizeStr, "M") || strings.HasSuffix(sizeStr, "MB") {
		multiplier = 1024 * 1024
		unit = "MB"
		if strings.HasSuffix(sizeStr, "M") {
			unit = "M"
		}
	} else if strings.HasSuffix(sizeStr, "K") || strings.HasSuffix(sizeStr, "KB") {
		multiplier = 1024
		unit = "KB"
		if strings.HasSuffix(sizeStr, "K") {
			unit = "K"
		}
	} else {
		// Check if it's just a number (bytes)
		if _, err := strconv.ParseUint(sizeStr, 10, 64); err == nil {
			multiplier = 1
			unit = ""
		} else {
			return 0, fmt.Errorf("invalid size format: %s. Must be in T, G, M, K or bytes (e.g. 1T, 10G, 500M, 1024K, 524288)", sizeStr)
		}
	}

	if unit != "" {
		sizeStr = strings.TrimSuffix(sizeStr, unit)
	}

	size, err := strconv.ParseUint(sizeStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size number: %w", err)
	}

	return size * multiplier, nil
}

func ParseSizeToGB(sizeStr string) (int, error) {
	bytes, err := ParseSize(sizeStr)
	if err != nil {
		return 0, err
	}
	if bytes == 0 {
		return 0, nil
	}
	// Convert bytes to GB, rounding up
	gb := (bytes + (1024*1024*1024 - 1)) / (1024 * 1024 * 1024)
	return int(gb), nil
}
