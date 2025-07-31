package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
)

type Config struct {
	Network           string        `yaml:"network"`
	Mnemonic          string        `yaml:"mnemonic"`
	DeploymentName    string        `yaml:"deployment_name"`
	MetaNodes         []uint32      `yaml:"meta_nodes"`
	DataNodes         []uint32      `yaml:"data_nodes"`
	Password          string        `yaml:"password"`
	MetaSizeGb        int           `yaml:"meta_size_gb"`
	DataSizeGb        int           `yaml:"data_size_gb"`
	MinShards         int           `yaml:"min_shards"`
	ExpectedShards    int           `yaml:"expected_shards"`
	ZdbRootPath       string        `yaml:"zdb_root_path"`
	QsfsMountpoint    string        `yaml:"qsfs_mountpoint"`
	CachePath         string        `yaml:"cache_path"`
	RetryInterval     time.Duration `yaml:"retry_interval"`
	DatabasePath      string        `yaml:"database_path"`
	ZdbRotateTime     time.Duration `yaml:"zdb_rotate_time"`
	ZdbConnectionType string        `yaml:"zdb_connection_type"`
	ZdbDataSize       string        `yaml:"zdb_data_size"`

	// For templates
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
	if mnemonic := os.Getenv("MNEMONIC"); mnemonic != "" {
		cfg.Mnemonic = mnemonic
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

	// Validate ZdbDataSize
	if size, err := parseSize(cfg.ZdbDataSize); err != nil {
		return nil, err
	} else if size < 524288 { // 0.5 MB
		return nil, fmt.Errorf("zdb_data_size cannot be smaller than 524288 bytes (0.5 MB)")
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

	if strings.HasSuffix(sizeStr, "G") {
		multiplier = 1024 * 1024 * 1024
		unit = "G"
	} else if strings.HasSuffix(sizeStr, "M") {
		multiplier = 1024 * 1024
		unit = "M"
	} else if strings.HasSuffix(sizeStr, "K") {
		multiplier = 1024
		unit = "K"
	} else {
		// Check if it's just a number (bytes)
		if _, err := strconv.ParseUint(sizeStr, 10, 64); err == nil {
			multiplier = 1
			unit = ""
		} else {
			return 0, fmt.Errorf("invalid size format: %s. Must be in G, M, K or bytes (e.g. 10G, 500M, 1024K, 524288)", sizeStr)
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
