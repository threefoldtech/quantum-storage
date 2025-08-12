package zstor

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/cosmos/go-bip39"
	"github.com/pkg/errors"
	"github.com/scottyeager/tfgrid-sdk-go/grid-client/workloads"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/config"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/util"
)

func GenerateRemoteConfig(cfg *config.Config, meta, data []*workloads.ZDB) string, err {
	key, err := keyFromMnemonic(cfg.Mnemonic, cfg.Password)
	if err != nil {
		return errors.Wrap(err, "failed to generate key from mnemonic")
	}

	size, err := util.ParseSize(cfg.ZdbDataSize)
	if err != nil {
		return errors.Wrap(err, "failed to parse zdb_data_size")
	}
	zdbDataSizeMb := size / (1024 * 1024)

	var configBuilder strings.Builder
	configBuilder.WriteString(fmt.Sprintf(`minimal_shards = %d
expected_shards = %d
redundant_groups = 0
redundant_nodes = 0
root = "/"
zdbfs_mountpoint = "%s"
socket = "/tmp/zstor.sock"
prometheus_port = 9200
zdb_data_dir_path = "%s/data/zdbfs-data/"
max_zdb_data_dir_size = %d

[compression]
algorithm = "snappy"

[meta]
type = "zdb"

[meta.config]
prefix = "zstor-meta"

[encryption]
algorithm = "AES"
key = "%s"

[meta.config.encryption]
algorithm = "AES"
key = "%s"`, cfg.MinShards, cfg.ExpectedShards, cfg.QsfsMountpoint, cfg.ZdbRootPath, zdbDataSizeMb, key, key))

	// Add meta backends
	for _, zdb := range meta {
		mappedIPs := util.MapIPs(zdb.IPs)
		ip, ok := mappedIPs[cfg.ZdbConnectionType]
		if !ok {
			return fmt.Errorf("ZDB connection type '%s' not supported on node for zdb %s", cfg.ZdbConnectionType, zdb.Name)
		}
		configBuilder.WriteString("\n\n[[meta.config.backends]]\n")
		configBuilder.WriteString(fmt.Sprintf("address = \"[%s]:9900\"\n", ip))
		configBuilder.WriteString(fmt.Sprintf("namespace = \"%s\"\n", zdb.Namespace))
		configBuilder.WriteString(fmt.Sprintf("password = \"%s\"", cfg.Password))
	}

	// Add data backends
	configBuilder.WriteString("\n\n[[groups]]")
	for _, zdb := range data {
		mappedIPs := util.MapIPs(zdb.IPs)
		ip, ok := mappedIPs[cfg.ZdbConnectionType]
		if !ok {
			return fmt.Errorf("ZDB connection type '%s' not supported on node for zdb %s", cfg.ZdbConnectionType, zdb.Name)
		}
		configBuilder.WriteString("\n\n[[groups.backends]]\n")
		configBuilder.WriteString(fmt.Sprintf("address = \"[%s]:9900\"\n", ip))
		configBuilder.WriteString(fmt.Sprintf("namespace = \"%s\"\n", zdb.Namespace))
		configBuilder.WriteString(fmt.Sprintf("password = \"%s\"", cfg.Password))
	}
	configBuilder.WriteString("\n")

	return configBuilder.String()
}

func keyFromMnemonic(mnemonic, password string) (string, error) {
	seed := bip39.NewSeed(mnemonic, password)
	hash := sha256.Sum256(seed)
	return fmt.Sprintf("%x", hash), nil
}
