package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/grid"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/service"
	"github.com/threefoldtech/tfgrid-sdk-go/grid-client/workloads"
)

var restoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "Restore a QSFS deployment from existing backends",
	Long: `This command restores a QSFS frontend on a new machine using existing
backend ZDBs. It discovers the existing deployments on the grid, generates the
necessary configuration, sets up the local services, and recovers the data.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runRestore(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(restoreCmd)
	restoreCmd.Flags().StringVarP(&ConfigOutPath, "out", "o", "/etc/zstor.toml", "Path to write generated zstor config")
}

func runRestore() error {
	cfg, err := LoadConfig(ConfigFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if cfg.DeploymentName == "" {
		return errors.New("deployment_name is required in config")
	}
	if cfg.Mnemonic == "" {
		return errors.New("mnemonic is required in config")
	}

	fmt.Println("Starting restoration process...")

	// 1. Connect to the grid and find deployments
	fmt.Println("Connecting to the grid to find existing deployments...")
	network := Network
	if cfg.Network != "" {
		network = cfg.Network
	}
	gridClient, err := grid.NewGridClient(cfg.Mnemonic, network)
	if err != nil {
		return errors.Wrap(err, "failed to create grid client")
	}

	twinID := uint64(gridClient.TwinID)
	contracts, err := grid.GetContracts(&gridClient, twinID)
	if err != nil {
		return errors.Wrapf(err, "failed to query for existing contracts for twin %d", twinID)
	}

	// 2. Filter deployments and load ZDBs
	fmt.Println("Filtering deployments and loading ZDB information...")
	var metaDeployments []*workloads.ZDB
	var dataDeployments []*workloads.ZDB

	for _, contractInfo := range contracts {
		name := contractInfo.DeploymentName
		expectedPrefix := fmt.Sprintf("%s_%d", cfg.DeploymentName, twinID)
		if !strings.HasPrefix(name, expectedPrefix) {
			continue
		}

		parts := strings.Split(name, "_")
		if len(parts) != 4 {
			continue
		}
		nodeType := parts[2]
		nodeID, err := strconv.ParseUint(parts[3], 10, 32)
		if err != nil {
			fmt.Printf("warn: could not parse nodeID from deployment name '%s': %v\n", name, err)
			continue
		}

		gridClient.State.StoreContractIDs(uint32(nodeID), uint64(contractInfo.Contract.ContractID))
		resZDB, err := gridClient.State.LoadZdbFromGrid(context.TODO(), uint32(nodeID), name, name)
		if err != nil {
			return errors.Wrapf(err, "failed to load deployed ZDB '%s' from node %d", name, nodeID)
		}

		if nodeType == "meta" {
			metaDeployments = append(metaDeployments, &resZDB)
			fmt.Printf("Found metadata ZDB '%s' on node %d\n", name, nodeID)
		} else if nodeType == "data" {
			dataDeployments = append(dataDeployments, &resZDB)
			fmt.Printf("Found data ZDB '%s' on node %d\n", name, nodeID)
		}
	}

	if len(metaDeployments) == 0 || len(dataDeployments) == 0 {
		return errors.New("no existing meta or data backends found for the given deployment name. cannot proceed with restore")
	}

	// 3. Generate zstor config
	fmt.Println("Generating zstor configuration file...")
	if err := generateRemoteConfig(cfg, metaDeployments, dataDeployments); err != nil {
		return errors.Wrap(err, "failed to generate zstor config")
	}

	// 4. Setup local machine (binaries, directories, services)
	fmt.Println("Setting up local machine...")
	serviceCfg := &service.Config{
		Network:        cfg.Network,
		Mnemonic:       cfg.Mnemonic,
		DeploymentName: cfg.DeploymentName,
		MetaNodes:      cfg.MetaNodes,
		DataNodes:      cfg.DataNodes,
		Password:       cfg.Password,
		MetaSizeGb:     cfg.MetaSizeGb,
		DataSizeGb:     cfg.DataSizeGb,
		MinShards:      cfg.MinShards,
		ExpectedShards: cfg.ExpectedShards,
		ZdbRootPath:    cfg.ZdbRootPath,
		QsfsMountpoint: cfg.QsfsMountpoint,
		CachePath:      cfg.CachePath,
		RetryInterval:  cfg.RetryInterval,
		DatabasePath:   cfg.DatabasePath,
		MetaBackends:   []service.Backend{},
		DataBackends:   []service.Backend{},
	}

	if err := service.Setup(serviceCfg, false); err != nil { // false for not local
		return errors.Wrap(err, "failed to perform local machine setup")
	}

	sm, err := service.NewServiceManager()
	if err != nil {
		return err
	}

	// Start zdb and zstor first
	for _, srv := range []string{"zdb", "zstor"} {
		if err := sm.StartService(srv); err != nil {
			return fmt.Errorf("failed to start service %s: %w", srv, err)
		}
	}

	// Wait for services to be ready
	if err := waitForServices(); err != nil {
		return errors.Wrap(err, "services did not start in time")
	}

	// 5. Perform recovery steps from script
	fmt.Println("Performing data recovery...")
	if err := recoverData(cfg); err != nil {
		return errors.Wrap(err, "failed to recover data")
	}

	// 6. Start zdbfs service
	fmt.Println("Starting ZDBFS service...")
	if err := sm.StartService("zdbfs"); err != nil {
		return errors.Wrap(err, "failed to start zdbfs service")
	}

	fmt.Println("Restore process completed successfully!")
	return nil
}

func waitForServices() error {
	fmt.Println("Waiting for services to initialize...")
	timeout := time.After(30 * time.Second)
	tick := time.NewTicker(1 * time.Second)
	defer tick.Stop()

	for {
		select {
		case <-timeout:
			return errors.New("timed out waiting for services")
		case <-tick.C:
			// Check for zdb
			zdbReady := false
			cmd := exec.Command("redis-cli", "-p", "9900", "PING")
			output, err := cmd.CombinedOutput()
			if err == nil && strings.TrimSpace(string(output)) == "PONG" {
				zdbReady = true
			}

			// Check for zstor
			zstorReady := false
			cmdZstor := exec.Command("zstor", "-c", ConfigOutPath, "test")
			if err := cmdZstor.Run(); err == nil {
				zstorReady = true
			}

			if zdbReady && zstorReady {
				fmt.Println("Services are ready.")
				return nil
			}
		}
	}
}

func recoverData(cfg *Config) error {
	// This function implements the logic from the recover.sh script.
	zstorCmd := func(args ...string) error {
		cmdArgs := append([]string{"-c", ConfigOutPath}, args...)
		cmd := exec.Command("zstor", cmdArgs...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		fmt.Printf("Running: %s\n", cmd.String())
		return cmd.Run()
	}

	// a. Create mount point (already done in setup)
	// b. Start zstor and zdb services (already done in setup)

	// c. Install redis-cli
	fmt.Println("Installing redis-cli...")
	if err := exec.Command("apt", "update").Run(); err != nil {
		return errors.Wrap(err, "apt update failed")
	}
	if err := exec.Command("apt", "install", "-y", "redis-tools").Run(); err != nil {
		return errors.Wrap(err, "failed to install redis-tools")
	}

	// d. Setup temp namespace
	fmt.Println("Setting up temporary namespace...")
	redisCmd := func(args ...string) error {
		cmdArgs := append([]string{"-p", "9900"}, args...)
		cmd := exec.Command("redis-cli", cmdArgs...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	// Check if namespace exists before creating
	nsInfoCmd := exec.Command("redis-cli", "-p", "9900", "NSINFO", "zdbfs-temp")
	output, err := nsInfoCmd.CombinedOutput()
	if err != nil && strings.Contains(string(output), "namespace not found") {
		fmt.Println("Temporary namespace 'zdbfs-temp' not found, creating it...")
		if err := redisCmd("NSNEW", "zdbfs-temp"); err != nil {
			return errors.Wrap(err, "failed to create temp namespace")
		}
	} else if err != nil {
		return errors.Wrapf(err, "failed to check for temp namespace: %s", string(output))
	} else {
		fmt.Println("Temporary namespace 'zdbfs-temp' already exists.")
	}

	if err := redisCmd("NSSET", "zdbfs-temp", "password", "hello"); err != nil {
		return errors.Wrap(err, "failed to set temp namespace password")
	}
	if err := redisCmd("NSSET", "zdbfs-temp", "public", "0"); err != nil {
		return errors.Wrap(err, "failed to set temp namespace public flag")
	}
	if err := redisCmd("NSSET", "zdbfs-temp", "mode", "seq"); err != nil {
		return errors.Wrap(err, "failed to set temp namespace mode")
	}

	// e. Recover metadata
	fmt.Println("Recovering metadata indexes...")
	if err := zstorCmd("retrieve", "--file", "/data/index/zdbfs-meta/zdb-namespace"); err != nil {
		return errors.Wrap(err, "failed to retrieve metadata namespace info")
	}

	for i := 0; ; i++ {
		filePath := fmt.Sprintf("/data/index/zdbfs-meta/i%d", i)
		err := zstorCmd("retrieve", "--file", filePath)
		if err != nil {
			fmt.Printf("Finished retrieving metadata indexes at i%d.\n", i-1)
			break
		}
	}

	fmt.Println("Retrieving latest metadata data file...")
	lastMetaIndex, err := findLastIndex("/data/index/zdbfs-meta")
	if err != nil {
		return errors.Wrap(err, "could not find last metadata index")
	}
	if err := zstorCmd("retrieve", "--file", fmt.Sprintf("/data/data/zdbfs-meta/d%d", lastMetaIndex)); err != nil {
		return errors.Wrap(err, "failed to retrieve latest metadata data file")
	}

	// f. Recover data
	fmt.Println("Recovering data indexes...")
	if err := zstorCmd("retrieve", "--file", "/data/index/zdbfs-data/zdb-namespace"); err != nil {
		return errors.Wrap(err, "failed to retrieve data namespace info")
	}

	for i := 0; ; i++ {
		filePath := fmt.Sprintf("/data/index/zdbfs-data/i%d", i)
		err := zstorCmd("retrieve", "--file", filePath)
		if err != nil {
			fmt.Printf("Finished retrieving data indexes at i%d.\n", i-1)
			break
		}
	}

	fmt.Println("Retrieving latest data data file...")
	lastDataIndex, err := findLastIndex("/data/index/zdbfs-data")
	if err != nil {
		return errors.Wrap(err, "could not find last data index")
	}
	if err := zstorCmd("retrieve", "--file", fmt.Sprintf("/data/data/zdbfs-data/d%d", lastDataIndex)); err != nil {
		return errors.Wrap(err, "failed to retrieve latest data data file")
	}

	// g. Start zdbfs service
	fmt.Println("Starting ZDBFS service...")
	sm, err := service.NewServiceManager()
	if err != nil {
		return err
	}
	if err := sm.StartService("zdbfs"); err != nil {
		return errors.Wrap(err, "failed to start zdbfs service")
	}

	return nil
}

func findLastIndex(dir string) (int, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return -1, err
	}

	lastIndex := -1
	for _, file := range files {
		if file.IsDir() || !strings.HasPrefix(file.Name(), "i") {
			continue
		}
		numPart := strings.TrimPrefix(file.Name(), "i")
		num, err := strconv.Atoi(numPart)
		if err == nil && num > lastIndex {
			lastIndex = num
		}
	}

	if lastIndex == -1 {
		return -1, fmt.Errorf("no index files found in %s", dir)
	}

	return lastIndex, nil
}
