package service

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"
)

// ServiceManager defines the interface for managing system services.
type ServiceManager interface {
	CreateServiceFiles(cfg *Config, isLocal bool) error
	StartService(name string) error
	EnableService(name string) error
	DisableService(name string) error
	StopService(name string) error
	DaemonReload() error
}

// Config mirrors the fields from cmd.Config needed for templates.
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
	MetaBackends   []Backend     `yaml:"-"`
	DataBackends   []Backend     `yaml:"-"`
}

type Backend struct {
	Address   string
	Namespace string
	Password  string
}

var (
	// TemplateAssets should be populated by the calling package
	TemplateAssets embed.FS
)

// NewServiceManager detects the init system and returns the appropriate ServiceManager.
func NewServiceManager() (ServiceManager, error) {
	if _, err := os.Stat("/proc/1/comm"); err == nil {
		if comm, err := os.ReadFile("/proc/1/comm"); err == nil {
			if strings.TrimSpace(string(comm)) == "systemd" {
				return &SystemdManager{}, nil
			}
		}
	}
	if _, err := exec.LookPath("zinit"); err == nil {
		return &ZinitManager{}, nil
	}
	return nil, fmt.Errorf("no supported init system found")
}

// SystemdManager implements ServiceManager for systemd.
type SystemdManager struct{}

func (s *SystemdManager) CreateServiceFiles(cfg *Config, isLocal bool) error {
	templatedServices := []string{"zdb", "zstor", "zdbfs", "quantumd"}
	for _, name := range templatedServices {
		err := renderTemplate(
			fmt.Sprintf("/etc/systemd/system/%s.service", name),
			fmt.Sprintf("%s.service.template", name),
			"systemd",
			cfg,
		)
		if err != nil {
			return err
		}
	}
	if isLocal {
		return s.createLocalSystemdBackends()
	}
	return nil
}

func (s *SystemdManager) StartService(name string) error {
	return exec.Command("systemctl", "start", name).Run()
}

// `zinit monitor` also has "now" behavior, so use the same here
func (s *SystemdManager) EnableService(name string) error {
	return exec.Command("systemctl", "enable", "--now", name).Run()
}

func (s *SystemdManager) DisableService(name string) error {
	return exec.Command("systemctl", "disable", name).Run()
}

func (s *SystemdManager) StopService(name string) error {
	return exec.Command("systemctl", "stop", name).Run()
}

func (s *SystemdManager) DaemonReload() error {
	return exec.Command("systemctl", "daemon-reload").Run()
}

func (s *SystemdManager) createLocalSystemdBackends() error {
	for i, port := range []int{9901, 9902, 9903, 9904} {
		service := fmt.Sprintf(`[Unit]
Description=Local ZDB Backend %d
After=network.target

[Service]
ExecStart=/usr/local/bin/zdb \
    --port=%d \
    --data=/data/data%d \
    --index=/data/index%d \
    --logfile=/var/log/zdb%d.log \
    --datasize 67108864 \
Restart=always
RestartSec=5
TimeoutStopSec=60

[Install]
WantedBy=multi-user.target`, i+1, port, i+1, i+1, i+1)

		path := fmt.Sprintf("/etc/systemd/system/zdb-back%d.service", i+1)
		if err := os.WriteFile(path, []byte(service), 0644); err != nil {
			return fmt.Errorf("failed to write service file %s: %w", path, err)
		}
	}
	return nil
}

// ZinitManager implements ServiceManager for zinit.
type ZinitManager struct{}

func (z *ZinitManager) CreateServiceFiles(cfg *Config, isLocal bool) error {
	services := []string{"zstor", "zdb", "zdbfs", "quantumd"}
	zinitDir := "/etc/zinit"

	if err := os.MkdirAll(zinitDir, 0755); err != nil {
		return fmt.Errorf("failed to create zinit directory: %w", err)
	}

	for _, name := range services {
		err := renderTemplate(
			filepath.Join(zinitDir, name+".yaml"),
			name+".yaml.template",
			"zinit",
			cfg,
		)
		if err != nil {
			return err
		}
	}
	if isLocal {
		return z.createLocalZinitBackends()
	}
	return nil
}

func (z *ZinitManager) StartService(name string) error {
	return exec.Command("zinit", "start", name).Run()
}

func (z *ZinitManager) EnableService(name string) error {
	// zinit monitors services by default when the file is present
	return exec.Command("zinit", "monitor", name).Run()
}

func (z *ZinitManager) DisableService(name string) error {
	return exec.Command("zinit", "forget", name).Run()
}

func (z *ZinitManager) StopService(name string) error {
	return exec.Command("zinit", "stop", name).Run()
}

func (z *ZinitManager) DaemonReload() error {
	// zinit automatically reloads on changes
	return nil
}

func (z *ZinitManager) createLocalZinitBackends() error {
	for i, port := range []int{9901, 9902, 9903, 9904} {
		service := fmt.Sprintf(`exec: |
  /usr/local/bin/zdb
    --port %d
    --data /data/data%d
    --index /data/index%d
    --logfile /var/log/zdb%d.log`, port, i+1, i+1, i+1)

		path := fmt.Sprintf("/etc/zinit/zdb-back%d.yaml", i+1)
		if err := os.WriteFile(path, []byte(service), 0644); err != nil {
			return fmt.Errorf("failed to write service file %s: %w", path, err)
		}
	}
	return nil
}

const (
	zdbfsVersion = "0.1.11"
	zdbVersion   = "2.0.8"
	zstorVersion = "0.5.0-rc.1"
)

// renderTemplate is a helper function to render templates.
func renderTemplate(destPath, templateName, serviceType string, cfg *Config) error {
	var templatePath string
	if serviceType == "" {
		templatePath = filepath.Join("assets/templates", templateName)
	} else {
		templatePath = filepath.Join("assets/templates", serviceType, templateName)
	}
	templateContent, err := TemplateAssets.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("failed to read embedded template %s: %w", templatePath, err)
	}

	tmpl, err := template.New(templateName).Parse(string(templateContent))
	if err != nil {
		return fmt.Errorf("failed to parse template %s: %w", templateName, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, cfg); err != nil {
		return fmt.Errorf("failed to execute template %s: %w", templateName, err)
	}

	return os.WriteFile(destPath, buf.Bytes(), 0644)
}

func Setup(cfg *Config, isLocal bool) error {
	if err := downloadBinaries(isLocal); err != nil {
		return fmt.Errorf("failed to download binaries: %w", err)
	}

	if err := createDirectories(cfg, isLocal); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	if isLocal {
		if err := generateLocalZstorConfig(); err != nil {
			return fmt.Errorf("failed to generate local zstor config: %w", err)
		}
	}
	sm, err := NewServiceManager()
	if err != nil {
		return err
	}
	if err := sm.CreateServiceFiles(cfg, isLocal); err != nil {
		return fmt.Errorf("failed to create service files: %w", err)
	}
	return sm.DaemonReload()
}

func getBinaryVersion(binaryPath string) (string, error) {
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		return "", fmt.Errorf("binary not found")
	}

	cmd := exec.Command(binaryPath, "--version")
	output, err := cmd.CombinedOutput()

	if err != nil && !strings.Contains(err.Error(), "exit status") {
		return "", fmt.Errorf("failed to get version: %w", err)
	}

	outputStr := string(output)

	cleanVersion := func(s string) string {
		re := regexp.MustCompile(`\x1b\[[0-9;]*m`)
		s = re.ReplaceAllString(s, "")
		s = strings.TrimSpace(s)
		s = strings.Trim(s, "'\"[]")
		return s
	}

	if strings.Contains(binaryPath, "zdb") && !strings.Contains(binaryPath, "zdbfs") {
		if strings.Contains(outputStr, "v") {
			parts := strings.Split(outputStr, "v")
			if len(parts) > 1 {
				versionPart := strings.Split(parts[1], " ")[0]
				return cleanVersion(strings.TrimPrefix(versionPart, "v")), nil
			}
		}
	} else if strings.Contains(binaryPath, "zdbfs") {
		if strings.Contains(outputStr, "zdbfs v") {
			parts := strings.Split(outputStr, "zdbfs v")
			if len(parts) > 1 {
				versionPart := strings.Split(parts[1], "\n")[0]
				return cleanVersion(versionPart), nil
			}
		}
	} else if strings.Contains(binaryPath, "zstor") {
		parts := strings.Fields(outputStr)
		if len(parts) >= 2 {
			return cleanVersion(strings.TrimPrefix(parts[1], "v")), nil
		}
	}

	return "", fmt.Errorf("unable to parse version from output: %s", outputStr)
}

func needsDownload(binaryName, expectedVersion string) (bool, error) {
	var binaryPath string
	if binaryName == "zstor" {
		binaryPath = "/bin/zstor"
	} else {
		binaryPath = "/usr/local/bin/" + binaryName
	}

	currentVersion, err := getBinaryVersion(binaryPath)
	if err != nil {
		if strings.Contains(err.Error(), "binary not found") {
			fmt.Printf("Binary %s not found, will download\n", binaryName)
			return true, nil
		}
		fmt.Printf("Error checking %s version: %v, will download\n", binaryName, err)
		return true, nil
	}

	if currentVersion == expectedVersion {
		fmt.Printf("Binary %s already has correct version %s, skipping download\n", binaryName, expectedVersion)
		return false, nil
	}

	fmt.Printf("Binary %s has version %s, expected %s, will download\n", binaryName, currentVersion, expectedVersion)
	return true, nil
}
func downloadBinaries(isLocal bool) error {
	binaries := map[string]string{
		"zdbfs": fmt.Sprintf("https://github.com/threefoldtech/0-db-fs/releases/download/v%s/zdbfs-%s-amd64-linux-static", zdbfsVersion, zdbfsVersion),
		"zdb":   fmt.Sprintf("https://github.com/threefoldtech/0-db/releases/download/v%s/zdb-%s-linux-amd64-static", zdbVersion, zdbVersion),
		"zstor": fmt.Sprintf("https://github.com/threefoldtech/0-stor_v2/releases/download/v%s/zstor_v2-x86_64-linux-musl", zstorVersion),
	}

	for name, url := range binaries {
		var expectedVersion string
		switch name {
		case "zdbfs":
			expectedVersion = zdbfsVersion
		case "zdb":
			expectedVersion = zdbVersion
		case "zstor":
			expectedVersion = zstorVersion
		}

		needsDL, err := needsDownload(name, expectedVersion)
		if err != nil {
			return fmt.Errorf("failed to check if %s needs download: %w", name, err)
		}

		if !needsDL {
			continue
		}

		fmt.Printf("Downloading %s v%s...\n", name, expectedVersion)
		dest := "/usr/local/bin/" + name
		if name == "zstor" {
			dest = "/bin/zstor"
		}

		cmd := exec.Command("wget", "-O", dest, url)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to download %s: %w", name, err)
		}

		if err := os.Chmod(dest, 0755); err != nil {
			return fmt.Errorf("failed to make %s executable: %w", name, err)
		}
	}

	return nil
}

func createDirectories(cfg *Config, isLocal bool) error {
	dirs := []string{
		cfg.QsfsMountpoint,
		cfg.ZdbRootPath,
		"/var/log",
	}
	if isLocal {
		for i := 1; i <= 4; i++ {
			dirs = append(dirs, fmt.Sprintf("/data/data%d", i), fmt.Sprintf("/data/index%d", i))
		}
	}

	for _, dir := range dirs {
		fmt.Printf("Creating directory %s...\n", dir)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}
	return nil
}

func generateLocalZstorConfig() error {
	fmt.Println("Generating local zstor config...")
	config := `minimal_shards = 2
expected_shards = 4
redundant_groups = 0
redundant_nodes = 0
root = "/"
zdbfs_mountpoint = "/mnt/"
socket = "/tmp/zstor.sock"
prometheus_port = 9200
zdb_data_dir_path = "/data/data/zdbfs-data/"
max_zdb_data_dir_size = 64

[compression]
algorithm = "snappy"

[meta]
type = "zdb"

[meta.config]
prefix = "zstor-meta"

[encryption]
algorithm = "AES"
key = "5ee8ab34dbd57df581d9aada2e433f3fae7e55833f9350fa74dfe196d0f5240f"

[meta.config.encryption]
algorithm = "AES"
key = "5ee8ab34dbd57df581d9aada2e433f3fae7e55833f9350fa74dfe196d0f5240f"

[[meta.config.backends]]
address = "127.0.0.1:9901"
namespace = "meta1"
password = "zdbpassword"

[[meta.config.backends]]
address = "127.0.0.1:9902"
namespace = "meta2"
password = "zdbpassword"

[[meta.config.backends]]
address = "127.0.0.1:9903"
namespace = "meta3"
password = "zdbpassword"

[[meta.config.backends]]
address = "127.0.0.1:9904"
namespace = "meta4"
password = "zdbpassword"

[[groups]]
[[groups.backends]]
address = "127.0.0.1:9901"
namespace = "data1"
password = "zdbpassword"

[[groups.backends]]
address = "127.0.0.1:9902"
namespace = "data2"
password = "zdbpassword"

[[groups.backends]]
address = "127.0.0.1:9903"
namespace = "data3"
password = "zdbpassword"

[[groups.backends]]
address = "127.0.0.1:9904"
namespace = "data4"
password = "zdbpassword"
`
	return os.WriteFile("/etc/zstor.toml", []byte(config), 0644)
}
