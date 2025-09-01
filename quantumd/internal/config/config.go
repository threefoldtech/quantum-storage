package config

import (
	"fmt"
	"os"
	"time"

	"github.com/threefoldtech/quantum-storage/quantumd/internal/util"
	"gopkg.in/yaml.v2"
)

type Config struct {
	Network              string        `yaml:"network"`
	Mnemonic             string        `yaml:"mnemonic"`
	RelayURL             string        `yaml:"relay_url"`
	RMBTimeout           time.Duration `yaml:"rmb_timeout"`
	DeploymentName       string        `yaml:"deployment_name"`
	MetaNodes            []uint32      `yaml:"meta_nodes"`
	DataNodes            []uint32      `yaml:"data_nodes"`
	Farms                []uint64      `yaml:"farms"`
	ExcludeNodes         []uint32      `yaml:"exclude_nodes"`
	Password             string        `yaml:"password"`
	MetaSize             string        `yaml:"meta_size"`
	DataSize             string        `yaml:"data_size"`
	TotalStorageSize     string        `yaml:"total_storage_size"`
	MinShards            int           `yaml:"min_shards"`
	ExpectedShards       int           `yaml:"expected_shards"`
	ZdbRootPath          string        `yaml:"zdb_root_path"`
	QsfsMountpoint       string        `yaml:"qsfs_mountpoint"`
	CachePath            string        `yaml:"cache_path"`
	RetryInterval        time.Duration `yaml:"retry_interval"`
	DatabasePath         string        `yaml:"database_path"`
	ZdbRotateTime        time.Duration `yaml:"zdb_rotate_time"`
	ZdbConnectionType    string        `yaml:"zdb_connection_type"`
	ZdbDataSize          string        `yaml:"zdb_data_size"`
	ZstorConfigPath      string        `yaml:"zstor_config_path"`
	PrometheusPort       int           `yaml:"prometheus_port"`
	MaxDeploymentRetries int           `yaml:"max_deployment_retries"`

	// For templates and internal use
	MetaSizeGb   int       `yaml:"-"`
	DataSizeGb   int       `yaml:"-"`
	ZdbfsSize    string    `yaml:"-"`
	MetaBackends []Backend `yaml:"-"`
	DataBackends []Backend `yaml:"-"`
}
type Backend struct {
	Address   string
	Namespace string
	Password  string
}

func LoadConfig(path string) (*Config, error) {
	f, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
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
		cfg.RMBTimeout = 30 * time.Second
	}

	if cfg.RelayURL == "" {
		cfg.RelayURL = "wss://relay.grid.tf"
		if cfg.Network != "main" {
			cfg.RelayURL = fmt.Sprintf("wss://relay.%s.grid.tf", cfg.Network)
		}
	}

	if cfg.MetaSize == "" {
		cfg.MetaSize = "1G"
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

	if cfg.ZdbRootPath == "" {
		cfg.ZdbRootPath = "/opt/zdb"
	}

	if cfg.QsfsMountpoint == "" {
		cfg.QsfsMountpoint = "/mnt/qsfs"
	}

	if cfg.ZstorConfigPath == "" {
		cfg.ZstorConfigPath = "/etc/zstor.toml"
	}

	if cfg.DeploymentName == "" {
		return nil, fmt.Errorf("deployment_name is required in config")
	}
	if cfg.Mnemonic == "" {
		return nil, fmt.Errorf("mnemonic is required in config or as environment variable MNEMONIC")
	}

	// Validate ZdbDataSize
	if size, err := util.ParseSize(cfg.ZdbDataSize); err != nil {
		return nil, err
	} else if size < 524288 { // 0.5 MB
		return nil, fmt.Errorf("zdb_data_size cannot be smaller than 524288 bytes (0.5 MB)")
	}

	// Parse MetaSize to GB
	if cfg.MetaSize != "" {
		metaSizeGb, err := util.ParseSizeToGB(cfg.MetaSize)
		if err != nil {
			return nil, fmt.Errorf("failed to parse meta_size: %w", err)
		}
		cfg.MetaSizeGb = metaSizeGb
	}

	// Calculate or parse DataSize to GB
	if cfg.DataSize != "" {
		dataSizeGb, err := util.ParseSizeToGB(cfg.DataSize)
		if err != nil {
			return nil, fmt.Errorf("failed to parse data_size: %w", err)
		}
		cfg.DataSizeGb = dataSizeGb
	} else if cfg.TotalStorageSize != "" {
		totalBytes, err := util.ParseSize(cfg.TotalStorageSize)
		if err != nil {
			return nil, fmt.Errorf("failed to parse total_storage_size: %w", err)
		}

		if cfg.ExpectedShards == 0 || cfg.MinShards == 0 {
			return nil, fmt.Errorf("expected_shards and min_shards must be set to calculate data backend size")
		}

		backendSizeBytes, err := util.ComputeBackendSize(int64(totalBytes), int64(cfg.ExpectedShards), int64(cfg.MinShards))
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
		totalBytes, err := util.ParseSize(cfg.TotalStorageSize)
		if err != nil {
			return nil, fmt.Errorf("failed to parse total_storage_size: %w", err)
		}
		cfg.ZdbfsSize = fmt.Sprintf("%d", totalBytes)
		fmt.Printf("Using total_storage_size for zdbfs size: %s\n", cfg.TotalStorageSize)
	} else if cfg.DataSizeGb > 0 && cfg.ExpectedShards > 0 && cfg.MinShards > 0 {
		// Otherwise, calculate it from the data backend size.
		backendSizeBytes := int64(cfg.DataSizeGb) * 1024 * 1024 * 1024
		totalStorage, err := util.ComputeTotalStorage(backendSizeBytes, int64(cfg.ExpectedShards), int64(cfg.MinShards))
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
