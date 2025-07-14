package cmd

import (
	"os"
	"time"

	"gopkg.in/yaml.v2"
)

type Config struct {
	Network        string        `yaml:"network"`
	Mnemonic       string        `yaml:"mnemonic"`
	MetaNodes      []uint32      `yaml:"meta_nodes"`
	DataNodes      []uint32      `yaml:"data_nodes"`
	ZdbPassword    string        `yaml:"zdb_password"`
	MetaSizeGb     int           `yaml:"meta_size_gb"`
	DataSizeGb     int           `yaml:"data_size_gb"`
	MinShards      int           `yaml:"min_shards"`
	ExpectedShards int           `yaml:"expected_shards"`
	ZdbRootPath    string        `yaml:"zdb_root_path"`
	QsfsMountpoint string        `yaml:"qsfs_mountpoint"`
	CachePath      string        `yaml:"cache_path"`
	RetryInterval  time.Duration `yaml:"retry_interval"`
	DatabasePath   string        `yaml:"database_path"`

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

	return &cfg, nil
}
