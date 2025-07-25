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
	Network        string        `yaml:"network"`
	Mnemonic       string        `yaml:"mnemonic"`
	DeploymentName string        `yaml:"deployment_name"`
	MetaNodes      []uint32      `yaml:"meta_nodes"`
	DataNodes      []uint32      `yaml:"data_nodes"`
	Password       string        `yaml:"password"`
	MetaSizeGb     int           `yaml:"meta_size_gb"`
	DataSizeGb     int           `yaml:"data_size_gb"`
	MinShards      int           `yaml:"min_shards"`
	ExpectedShards int           `yaml:"expected_shards"`
	ZdbRootPath    string        `yaml:"zdb_root_path"`
	QsfsMountpoint string        `yaml:"qsfs_mountpoint"`
	CachePath      string        `yaml:"cache_path"`
	RetryInterval  time.Duration `yaml:"retry_interval"`
	DatabasePath   string        `yaml:"database_path"`
	ZdbRotateTime  time.Duration `yaml:"zdb_rotate_time"`
	ZdbConnectionType string `yaml:"zdb_connection_type"`
	ZdbDataSize       string `yaml:"zdb_data_size"`

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

	return &cfg, nil
}

func parseSize(sizeStr string) (int, error) {
	sizeStr = strings.ToUpper(strings.TrimSpace(sizeStr))
	if sizeStr == "" {
		return 0, nil
	}

	var multiplier int
	if strings.HasSuffix(sizeStr, "G") {
		multiplier = 1024
		sizeStr = strings.TrimSuffix(sizeStr, "G")
	} else if strings.HasSuffix(sizeStr, "M") {
		multiplier = 1
		sizeStr = strings.TrimSuffix(sizeStr, "M")
	} else {
		return 0, fmt.Errorf("invalid size format: %s. Must be in M or G (e.g. 10G, 500M)", sizeStr)
	}

	size, err := strconv.Atoi(sizeStr)
	if err != nil {
		return 0, fmt.Errorf("invalid size number: %w", err)
	}

	return size * multiplier, nil
}
