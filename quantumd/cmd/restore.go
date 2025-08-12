package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
	"github.com/scottyeager/tfgrid-sdk-go/grid-client/workloads"
	"github.com/spf13/cobra"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/config"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/grid"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/hook"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/service"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/zstor"
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
	if err := DownloadBinaries(); err != nil {
		return fmt.Errorf("failed to download binaries: %w", err)
	}

	cfg, err := config.LoadConfig(ConfigFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if _, err := CreateDirectories(cfg, false); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	fmt.Println("Starting restoration process...")

	gridClient, err := grid.NewGridClient(cfg.Network, cfg.Mnemonic, cfg.RelayURL, cfg.RMBTimeout)
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
	zstorConfig, err := zstor.GenerateRemoteConfig(cfg, metaDeployments, dataDeployments)
	if err != nil {
		return errors.Wrap(err, "failed to generate remote config")
	}

	// Write config file
	if err := os.WriteFile(ConfigOutPath, []byte(zstorConfig), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	// 4. Setup hook symlink
	if err := hook.SetupSymlink(); err != nil {
		return fmt.Errorf("failed to setup hook symlink: %w", err)
	}

	// 5. Setup local machine (binaries, directories, services)
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
		ZdbDataSize:    cfg.ZdbDataSize,
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

	// Manually start zstor and zdb for recovery
	fmt.Println("Starting temporary zstor and zdb for recovery...")
	if err := sm.EnableService("zstor"); err != nil {
		return fmt.Errorf("failed to start temporary zstor service: %w", err)
	}

	zdbCmd := exec.Command("/usr/local/bin/zdb",
		"--index", cfg.ZdbRootPath+"/index",
		"--data", cfg.ZdbRootPath+"/data",
		"--logfile", "/var/log/zdb.log",
		"--datasize", cfg.ZdbDataSize,
		"--hook", "/usr/local/bin/quantumd-hook",
	)
	zdbCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var out bytes.Buffer
	zdbCmd.Stdout = &out
	zdbCmd.Stderr = &out
	if err := zdbCmd.Start(); err != nil {
		return fmt.Errorf("failed to start temporary zdb process: %w", err)
	}

	// Defer cleanup of temporary services
	defer func() {
		fmt.Println("Cleaning up temporary services...")
		// Kill the entire process group of zdb
		if err := syscall.Kill(-zdbCmd.Process.Pid, syscall.SIGKILL); err != nil {
			fmt.Printf("warn: failed to kill temporary zdb process group: %v\n", err)
		}
	}()

	// Wait for services to be ready
	if err := waitForServices(); err != nil {
		fmt.Printf("zdb command output:\n%s\n", out.String())
		return errors.Wrap(err, "services did not start in time")
	}

	// 5. Perform recovery steps from script
	fmt.Println("Performing data recovery...")
	if err := recoverData(cfg); err != nil {
		return errors.Wrap(err, "failed to recover data")
	}

	// Cleanup is handled by defer. Now start the final services.
	fmt.Println("Recovery successful. Starting all system services...")

	servicesToStart := []string{"zdb", "zdbfs", "quantumd"}
	for _, srv := range servicesToStart {
		fmt.Printf("Enabling and starting service %s...\n", srv)
		if err := sm.EnableService(srv); err != nil {
			fmt.Printf("warn: failed to enable service %s: %v\n", srv, err)
		}
		if err := sm.StartService(srv); err != nil {
			// Don't fail the whole process, just log a warning
			fmt.Printf("warn: failed to start service %s: %v\n", srv, err)
		}
	}

	fmt.Println("Restore process completed successfully!")
	return nil
}

func waitForServices() error {
	fmt.Println("Waiting for services to initialize...")
	timeout := time.After(30 * time.Second)
	tick := time.NewTicker(1 * time.Second)
	defer tick.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:9900",
	})
	defer rdb.Close()

	for {
		select {
		case <-timeout:
			return errors.New("timed out waiting for services")
		case <-tick.C:
			// Check for zdb
			zdbReady := false
			_, err := rdb.Ping(ctx).Result()
			if err == nil {
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

func recoverData(cfg *config.Config) error {
	// This function implements the logic from the recover.sh script.
	zstorCmd := func(args ...string) error {
		cmdArgs := append([]string{"-c", ConfigOutPath}, args...)
		cmd := exec.Command("zstor", cmdArgs...)
		fmt.Printf("Running: %s\n", cmd.String())
		output, err := cmd.CombinedOutput()
		if err != nil {
			// Check for the specific "entity not found" error which is expected at the end of retrieval
			if strings.Contains(string(output), "entity not found") {
				return fmt.Errorf("not found") // Special error to signal end of loop
			}
			// For other errors, print the full output for context
			fmt.Print(string(output))
			return err
		}
		fmt.Print(string(output))
		return nil
	}

	fmt.Println("Setting up temporary namespace...")
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:9900",
	})
	defer rdb.Close()

	// Check if namespace exists before creating
	_, err := rdb.Do(ctx, "NSINFO", "zdbfs-temp").Result()
	if err != nil {
		if redis.HasErrorPrefix(err, "Namespace not found") {
			fmt.Println("Temporary namespace 'zdbfs-temp' not found, creating it...")
			if err := rdb.Do(ctx, "NSNEW", "zdbfs-temp").Err(); err != nil {
				return errors.Wrap(err, "failed to create temp namespace")
			}
		} else {
			return errors.Wrapf(err, "failed to check for temp namespace")
		}
	} else {
		fmt.Println("Temporary namespace 'zdbfs-temp' already exists.")
	}

	if err := rdb.Do(ctx, "NSSET", "zdbfs-temp", "password", "hello").Err(); err != nil {
		return errors.Wrap(err, "failed to set temp namespace password")
	}
	if err := rdb.Do(ctx, "NSSET", "zdbfs-temp", "public", "0").Err(); err != nil {
		return errors.Wrap(err, "failed to set temp namespace public flag")
	}
	if err := rdb.Do(ctx, "NSSET", "zdbfs-temp", "mode", "seq").Err(); err != nil {
		return errors.Wrap(err, "failed to set temp namespace mode")
	}

	metaIndexDir := fmt.Sprintf("%s/index/zdbfs-meta", cfg.ZdbRootPath)
	metaDataDir := fmt.Sprintf("%s/data/zdbfs-meta", cfg.ZdbRootPath)
	dataIndexDir := fmt.Sprintf("%s/index/zdbfs-data", cfg.ZdbRootPath)
	dataDataDir := fmt.Sprintf("%s/data/zdbfs-data", cfg.ZdbRootPath)

	fmt.Println("Recovering metadata indexes...")
	if err := zstorCmd("retrieve", "--file", fmt.Sprintf("%s/zdb-namespace", metaIndexDir)); err != nil {
		// If the namespace file itself isn't found, it's a real error.
		if errors.Is(err, os.ErrNotExist) || strings.Contains(err.Error(), "not found") {
			fmt.Println("No metadata namespace info found, which might be okay. Continuing...")
		} else {
			return errors.Wrap(err, "failed to retrieve metadata namespace info")
		}
	}

	for i := 0; ; i++ {
		filePath := fmt.Sprintf("%s/i%d", metaIndexDir, i)
		err := zstorCmd("retrieve", "--file", filePath)
		if err != nil {
			if err.Error() == "not found" {
				fmt.Printf("Finished retrieving metadata indexes at i%d.\n", i-1)
				break
			}
			return errors.Wrapf(err, "error retrieving metadata index %s", filePath)
		}
	}

	fmt.Println("Retrieving latest metadata data file...")
	lastMetaIndex, err := findLastIndex(metaIndexDir)
	if err != nil {
		fmt.Printf("Could not find last metadata index, this might be okay if no data was written: %v\n", err)
	} else {
		if err := zstorCmd("retrieve", "--file", fmt.Sprintf("%s/d%d", metaDataDir, lastMetaIndex)); err != nil {
			if err.Error() != "not found" {
				return errors.Wrap(err, "failed to retrieve latest metadata data file")
			}
			fmt.Println("Latest metadata data file not found, which might be okay.")
		}
	}

	fmt.Println("Recovering data indexes...")
	if err := zstorCmd("retrieve", "--file", fmt.Sprintf("%s/zdb-namespace", dataIndexDir)); err != nil {
		if errors.Is(err, os.ErrNotExist) || strings.Contains(err.Error(), "not found") {
			fmt.Println("No data namespace info found, which might be okay. Continuing...")
		} else {
			return errors.Wrap(err, "failed to retrieve data namespace info")
		}
	}

	for i := 0; ; i++ {
		filePath := fmt.Sprintf("%s/i%d", dataIndexDir, i)
		err := zstorCmd("retrieve", "--file", filePath)
		if err != nil {
			if err.Error() == "not found" {
				fmt.Printf("Finished retrieving data indexes at i%d.\n", i-1)
				break
			}
			return errors.Wrapf(err, "error retrieving data index %s", filePath)
		}
	}

	fmt.Println("Retrieving latest data data file...")
	lastDataIndex, err := findLastIndex(dataIndexDir)
	if err != nil {
		fmt.Printf("Could not find last data index, this might be okay if no data was written: %v\n", err)
	} else {
		if err := zstorCmd("retrieve", "--file", fmt.Sprintf("%s/d%d", dataDataDir, lastDataIndex)); err != nil {
			if err.Error() != "not found" {
				return errors.Wrap(err, "failed to retrieve latest data data file")
			}
			fmt.Println("Latest data data file not found, which might be okay.")
		}
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
