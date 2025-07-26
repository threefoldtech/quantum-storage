package service

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	ZdbDataSize    string        `yaml:"zdb_data_size"`
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
		return s.createLocalSystemdBackends(cfg.ZdbDataSize)
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

func (s *SystemdManager) createLocalSystemdBackends(zdbDataSize string) error {
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
    --datasize %s \
Restart=always
RestartSec=5
TimeoutStopSec=60

[Install]
WantedBy=multi-user.target`, i+1, port, i+1, i+1, i+1, zdbDataSize)

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
	sm, err := NewServiceManager()
	if err != nil {
		return err
	}
	if err := sm.CreateServiceFiles(cfg, isLocal); err != nil {
		return fmt.Errorf("failed to create service files: %w", err)
	}
	return sm.DaemonReload()
}
