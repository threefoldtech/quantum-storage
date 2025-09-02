package zstor

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/cosmos/go-bip39"
	"github.com/pkg/errors"
	"github.com/scottyeager/tfgrid-sdk-go/grid-client/workloads"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/config"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/util"
)

type BackendConfig struct {
	Address   string `toml:"address"`
	Namespace string `toml:"namespace"`
	Password  string `toml:"password"`
}

type CompressionConfig struct {
	Algorithm string `toml:"algorithm"`
}

type MetaConfig struct {
	Type   string       `toml:"type"`
	Config MetaZdbConfig `toml:"config"`
}

type MetaZdbConfig struct {
	Prefix     string         `toml:"prefix"`
	Backends   []BackendConfig `toml:"backends"`
	Encryption EncryptionConfig `toml:"encryption"`
}

type EncryptionConfig struct {
	Algorithm string `toml:"algorithm"`
	Key       string `toml:"key"`
}

type GroupConfig struct {
	Backends []BackendConfig `toml:"backends"`
}

type ZstorConfig struct {
	MinimalShards        int              `toml:"minimal_shards"`
	ExpectedShards       int              `toml:"expected_shards"`
	RedundantGroups      int              `toml:"redundant_groups"`
	RedundantNodes       int              `toml:"redundant_nodes"`
	Root                 string           `toml:"root"`
	ZdbfsMountpoint      string           `toml:"zdbfs_mountpoint"`
	Socket               string           `toml:"socket"`
	PrometheusPort       int              `toml:"prometheus_port"`
	ZdbDataDirPath       string           `toml:"zdb_data_dir_path"`
	MaxZdbDataDirSize    int64            `toml:"max_zdb_data_dir_size"`
	Compression          CompressionConfig `toml:"compression"`
	Meta                 MetaConfig        `toml:"meta"`
	Encryption           EncryptionConfig  `toml:"encryption"`
	Groups               []GroupConfig     `toml:"groups"`
}

// LoadConfig loads a ZstorConfig from a TOML file
func LoadConfig(path string) (*ZstorConfig, error) {
	var config ZstorConfig
	if _, err := toml.DecodeFile(path, &config); err != nil {
		return nil, fmt.Errorf("failed to parse zstor config: %w", err)
	}
	return &config, nil
}

// SaveConfig saves a ZstorConfig to a TOML file
func (cfg *ZstorConfig) SaveConfig(path string) error {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(cfg); err != nil {
		return errors.Wrap(err, "failed to encode zstor config to TOML")
	}

	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		return errors.Wrap(err, "failed to write zstor config to file")
	}

	return nil
}

func GenerateRemoteConfig(cfg *config.Config, meta, data []workloads.Deployment) (string, error) {
	key, err := keyFromMnemonic(cfg.Mnemonic, cfg.Password)
	if err != nil {
		return "", errors.Wrap(err, "failed to generate key from mnemonic")
	}

	size, err := util.ParseSize(cfg.ZdbDataSize)
	if err != nil {
		return "", errors.Wrap(err, "failed to parse zdb_data_size")
	}
	zdbDataSizeMb := size / (1024 * 1024)

	// Prepare meta backends
	var metaBackends []BackendConfig
	for _, deployment := range meta {
		zdb := deployment.Zdbs[0]
		if len(zdb.IPs) == 0 {
			return "", fmt.Errorf("Error parsing deployment info for zdb %s: no IPs found", zdb.Name)
		}
		mappedIPs := util.MapIPs(zdb.IPs)
		ip, ok := mappedIPs[cfg.ZdbConnectionType]
		if !ok {
			return "", fmt.Errorf("ZDB connection type '%s' not supported on node for zdb %s", cfg.ZdbConnectionType, zdb.Name)
		}
		metaBackends = append(metaBackends, BackendConfig{
			Address:   fmt.Sprintf("[%s]:9900", ip),
			Namespace: zdb.Namespace,
			Password:  cfg.Password,
		})
	}

	// Prepare data backends
	var dataBackends []BackendConfig
	for _, deployment := range data {
		zdb := deployment.Zdbs[0]
		if len(zdb.IPs) == 0 {
			return "", fmt.Errorf("Error parsing deployment info for zdb %s: no IPs found", zdb.Name)
		}
		mappedIPs := util.MapIPs(zdb.IPs)
		ip, ok := mappedIPs[cfg.ZdbConnectionType]
		if !ok {
			return "", fmt.Errorf("ZDB connection type '%s' not supported on node for zdb %s", cfg.ZdbConnectionType, zdb.Name)
		}
		dataBackends = append(dataBackends, BackendConfig{
			Address:   fmt.Sprintf("[%s]:9900", ip),
			Namespace: zdb.Namespace,
			Password:  cfg.Password,
		})
	}

	// Create config struct
	zstorConfig := ZstorConfig{
		MinimalShards:     cfg.MinShards,
		ExpectedShards:    cfg.ExpectedShards,
		RedundantGroups:   0,
		RedundantNodes:    0,
		Root:              "",
		ZdbfsMountpoint:   cfg.QsfsMountpoint,
		Socket:            "/tmp/zstor.sock",
		PrometheusPort:    9200,
		ZdbDataDirPath:    fmt.Sprintf("%s/data/zdbfs-data/", cfg.ZdbRootPath),
		MaxZdbDataDirSize: int64(zdbDataSizeMb),
		Compression: CompressionConfig{
			Algorithm: "snappy",
		},
		Meta: MetaConfig{
			Type: "zdb",
			Config: MetaZdbConfig{
				Prefix: "zstor-meta",
				Backends: metaBackends,
				Encryption: EncryptionConfig{
					Algorithm: "AES",
					Key:       key,
				},
			},
		},
		Encryption: EncryptionConfig{
			Algorithm: "AES",
			Key:       key,
		},
		Groups: []GroupConfig{
			{Backends: dataBackends},
		},
	}

	// Serialize to TOML
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(zstorConfig); err != nil {
		return "", errors.Wrap(err, "failed to encode zstor config to TOML")
	}

	return buf.String(), nil
}

func keyFromMnemonic(mnemonic, password string) (string, error) {
	seed := bip39.NewSeed(mnemonic, password)
	hash := sha256.Sum256(seed)
	return fmt.Sprintf("%x", hash), nil
}
