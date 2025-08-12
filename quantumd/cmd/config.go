package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/threefoldtech/quantum-storage/quantumd/internal/config"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/math"
	"gopkg.in/yaml.v2"
)

func LoadConfig(path string) (*config.Config, error) {
	f, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg config.Config
	err = yaml.Unmarshal(f, &cfg)
	if err != nil {
		return nil, err
	}

	// Override with environment variables if they are set
	if network := os.Getenv("NETWORK"); network != "" {
		cfg.Network = network
	}

	if mnemonic := os.Getenv("MNEMONIC"); mnemonic != "" {
		cfg.Mnemonic = mnemonic
	}

	if cfg.RMBTimeout == 0 {
		cfg.RMBTimeout = 10 * time.Second
	}

	if cfg.RelayURL == "" {
		cfg.RelayURL = "wss://relay.grid.tf"
		if cfg.Network != "main" {
			cfg.RelayURL = fmt.Sprintf("wss://relay.%s.grid.tf", cfg.Network)
		}
	}

	if cfg.ZdbRotateTime == 0 {
		cfg.ZdbRotateTime = cfg.RetryInterval
	}
	if cfg.ZdbConnectionType == "" {
		cfg.ZdbConnectionType = "mycelium"
	}
	if cfg.ZdbDataSize == "" {
		cfg.ZdbDataSize = "64M"
	}
	if cfg.PrometheusPort == 0 {
		cfg.PrometheusPort = 9092
	}
	if cfg.MaxDeploymentRetries == 0 {
		cfg.MaxDeploymentRetries = 5
	}

	if cfg.DeploymentName == "" {
		return nil, fmt.Errorf("deployment_name is required in config")
	}
	if cfg.Mnemonic == "" {
		return nil, fmt.Errorf("mnemonic is required in config or as environment variable MNEMONIC")
	}

	// Validate ZdbDataSize
	if size, err := parseSize(cfg.ZdbDataSize); err != nil {
		return nil, err
	} else if size < 524288 { // 0.5 MB
		return nil, fmt.Errorf("zdb_data_size cannot be smaller than 524288 bytes (0.5 MB)")
	}

	// Parse MetaSize to GB
	if cfg.MetaSize != "" {
		metaSizeGb, err := parseSizeToGB(cfg.MetaSize)
		if err != nil {
			return nil, fmt.Errorf("failed to parse meta_size: %w", err)
		}
		cfg.MetaSizeGb = metaSizeGb
	}

	// Calculate or parse DataSize to GB
	if cfg.DataSize != "" {
		dataSizeGb, err := parseSizeToGB(cfg.DataSize)
		if err != nil {
			return nil, fmt.Errorf("failed to parse data_size: %w", err)
		}
		cfg.DataSizeGb = dataSizeGb
	} else if cfg.TotalStorageSize != "" {
		totalBytes, err := parseSize(cfg.TotalStorageSize)
		if err != nil {
			return nil, fmt.Errorf("failed to parse total_storage_size: %w", err)
		}

		if cfg.ExpectedShards == 0 || cfg.MinShards == 0 {
			return nil, fmt.Errorf("expected_shards and min_shards must be set to calculate data backend size")
		}

		backendSizeBytes, err := math.ComputeBackendSize(int64(totalBytes), int64(cfg.ExpectedShards), int64(cfg.MinShards))
		if err != nil {
			return nil, fmt.Errorf("failed to compute backend size: %w", err)
		}

		// Convert bytes to GB, rounding up
		backendSizeGB := (backendSizeBytes + (1024*1024*1024 - 1)) / (1024 * 1024 * 1024)
		cfg.DataSizeGb = int(backendSizeGB)
		fmt.Printf("Calculated data backend size: %d GB per backend\n", cfg.DataSizeGb)
	}

	// If TotalStorageSize is present, use it to calculate zdbfs_size
	if cfg.TotalStorageSize != "" {
		totalBytes, err := parseSize(cfg.TotalStorageSize)
		if err != nil {
			return nil, fmt.Errorf("failed to parse total_storage_size: %w", err)
		}
		cfg.ZdbfsSize = fmt.Sprintf("%d", totalBytes)
		fmt.Printf("Using total_storage_size for zdbfs size: %s\n", cfg.TotalStorageSize)
	} else if cfg.DataSizeGb > 0 && cfg.ExpectedShards > 0 && cfg.MinShards > 0 {
		// Otherwise, calculate it from the data backend size.
		backendSizeBytes := int64(cfg.DataSizeGb) * 1024 * 1024 * 1024
		totalStorage, err := math.ComputeTotalStorage(backendSizeBytes, int64(cfg.ExpectedShards), int64(cfg.MinShards))
		if err != nil {
			return nil, fmt.Errorf("failed to compute total storage for zdbfs size: %w", err)
		}
		cfg.ZdbfsSize = fmt.Sprintf("%d", totalStorage)
		fmt.Printf("Calculated zdbfs size: %s\n", cfg.ZdbfsSize)
	} else {
		return nil, fmt.Errorf("cannot calculate zdbfs_size without data_size or total_storage_size, expected_shards, and min_shards")
	}

	return &cfg, nil
}

func parseSize(sizeStr string) (uint64, error) {
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

func parseSizeToGB(sizeStr string) (int, error) {
	bytes, err := parseSize(sizeStr)
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
