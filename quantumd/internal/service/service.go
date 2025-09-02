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

	"github.com/scottyeager/tfgrid-sdk-go/grid-client/workloads"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/config"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/util"
)

// ServiceManager defines the interface for managing system services.
type ServiceManager interface {
	CreateServiceFiles(cfg *config.Config, metaBackends, dataBackends []workloads.Deployment, isLocal bool) error
	StartService(name string) error
	EnableService(name string) error
	DisableService(name string) error
	StopService(name string) error
	DaemonReload() error
	ServiceExists(name string) (bool, error)
	ServiceIsRunning(name string) (bool, error)
}

// ManagedServices is the list of services quantumd manages
var ManagedServices = []string{"zdb", "zstor", "zdbfs", "quantumd"}

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

func (s *SystemdManager) CreateServiceFiles(cfg *config.Config, metaBackends, dataBackends []workloads.Deployment, isLocal bool) error {
	// Convert grid deployments to service backends for templates
	serviceMetaBackends := convertDeploymentsToBackends(cfg, metaBackends)
	serviceDataBackends := convertDeploymentsToBackends(cfg, dataBackends)
	
	// Create a copy of the config with backends for template rendering
	cfgWithBackends := *cfg
	cfgWithBackends.MetaBackends = serviceMetaBackends
	cfgWithBackends.DataBackends = serviceDataBackends
	
	for _, name := range ManagedServices {
		err := renderTemplate(
			fmt.Sprintf("/etc/systemd/system/%s.service", name),
			fmt.Sprintf("%s.service.template", name),
			"systemd",
			&cfgWithBackends,
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

// convertDeploymentsToBackends converts grid deployments to service backends
func convertDeploymentsToBackends(cfg *config.Config, deployments []workloads.Deployment) []config.Backend {
	var backends []config.Backend
	for _, deployment := range deployments {
		if len(deployment.Zdbs) == 0 {
			continue
		}
		zdb := deployment.Zdbs[0]
		if len(zdb.IPs) == 0 {
			continue
		}
		mappedIPs := util.MapIPs(zdb.IPs)
		ip, ok := mappedIPs[cfg.ZdbConnectionType]
		if !ok {
			continue
		}
		backends = append(backends, config.Backend{
			Address:   fmt.Sprintf("[%s]:9900", ip),
			Namespace: zdb.Namespace,
			Password:  cfg.Password,
		})
	}
	return backends
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

func (s *SystemdManager) ServiceExists(name string) (bool, error) {
	_, err := os.Stat(fmt.Sprintf("/etc/systemd/system/%s.service", name))
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

func (s *SystemdManager) ServiceIsRunning(name string) (bool, error) {
	cmd := exec.Command("systemctl", "is-active", "--quiet", name)
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	if _, ok := err.(*exec.ExitError); ok {
		return false, nil
	}
	return false, err
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
			return fmt.Errorf("failed to write service file %s: %%w", path, err)
		}
	}
	return nil
}

// ZinitManager implements ServiceManager for zinit.
type ZinitManager struct{}

func (z *ZinitManager) CreateServiceFiles(cfg *config.Config, metaBackends, dataBackends []workloads.Deployment, isLocal bool) error {
	zinitDir := "/etc/zinit"

	if err := os.MkdirAll(zinitDir, 0755); err != nil {
		return fmt.Errorf("failed to create zinit directory: %w", err)
	}
	
	// Convert grid deployments to service backends for templates
	serviceMetaBackends := convertDeploymentsToBackends(cfg, metaBackends)
	serviceDataBackends := convertDeploymentsToBackends(cfg, dataBackends)
	
	// Create a copy of the config with backends for template rendering
	cfgWithBackends := *cfg
	cfgWithBackends.MetaBackends = serviceMetaBackends
	cfgWithBackends.DataBackends = serviceDataBackends

	for _, name := range ManagedServices {
		err := renderTemplate(
			filepath.Join(zinitDir, name+".yaml"),
			name+".yaml.template",
			"zinit",
			&cfgWithBackends,
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

func (z *ZinitManager) ServiceExists(name string) (bool, error) {
	_, err := os.Stat(filepath.Join("/etc/zinit", name+".yaml"))
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

func (z *ZinitManager) ServiceIsRunning(name string) (bool, error) {
	cmd := exec.Command("zinit", "status", name)
	output, err := cmd.Output()
	if err != nil {
		// If the service does not exist, zinit status returns an error.
		// We can treat this as "not running".
		if strings.Contains(string(output), "no such service") || strings.Contains(string(err.Error()), "exit status 1") {
			return false, nil
		}
		return false, err
	}

	return strings.Contains(string(output), "state: Running"), nil
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
			return fmt.Errorf("failed to write service file %s: %%w", path, err)
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
func renderTemplate(destPath, templateName, serviceType string, cfg *config.Config) error {
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

func Setup(cfg *config.Config, metaBackends, dataBackends []workloads.Deployment, isLocal bool) error {
	sm, err := NewServiceManager()
	if err != nil {
		return err
	}
	if err := sm.CreateServiceFiles(cfg, metaBackends, dataBackends, isLocal); err != nil {
		return fmt.Errorf("failed to create service files: %w", err)
	}
	return sm.DaemonReload()
}

func StartServiceByName(name string) error {
	sm, err := NewServiceManager()
	if err != nil {
		return fmt.Errorf("failed to get service manager: %%w", err)
	}
	fmt.Printf("Starting %s...\n", name)
	if err := sm.StartService(name); err != nil {
		return fmt.Errorf("failed to start service %s: %%w", name, err)
	}
	fmt.Printf("Service %s started.\n", name)
	return nil
}

func StartAllServices() {
	fmt.Println("Starting all services...")
	for _, s := range ManagedServices {
		if err := StartServiceByName(s); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to start service %s: %%v\n", s, err)
		}
	}
}

func StopServiceByName(name string) error {
	sm, err := NewServiceManager()
	if err != nil {
		return fmt.Errorf("failed to get service manager: %%w", err)
	}

	running, err := sm.ServiceIsRunning(name)
	if err != nil {
		return fmt.Errorf("failed to check status of service %s: %%w", name, err)
	}

	if running {
		fmt.Printf("Stopping %s...\n", name)
		if err := sm.StopService(name); err != nil {
			return fmt.Errorf("failed to stop service %s: %%w", name, err)
		}
		fmt.Printf("Service %s stopped.\n", name)
	} else {
		fmt.Printf("Service %s is not running.\n", name)
	}
	return nil
}

func StopAllServices() {
	fmt.Println("Stopping all services...")
	for _, s := range ManagedServices {
		if err := StopServiceByName(s); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to stop service %s: %%v\n", s, err)
		}
	}
}
