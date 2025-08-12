package config

import "time"

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
