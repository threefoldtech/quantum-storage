package zstor

import (
	"bufio"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/prometheus/client_golang/prometheus"
)

// BackendStatus represents the status of a zstor backend
type BackendStatus struct {
	Address     string
	BackendType string
	Namespace   string
	IsAlive     bool
	LastSeen    time.Time
}

// ZstorConfig represents the structure of the zstor TOML config file
type ZstorConfig struct {
	PrometheusPort int `toml:"prometheus_port"`
}

// MetricsScraper handles scraping and storing zstor backend status metrics
type MetricsScraper struct {
	configPath     string
	backendStatus  map[string]*BackendStatus
	statusGauge    *prometheus.GaugeVec
	lastScrapeTime prometheus.Gauge
}

// NewMetricsScraper creates a new metrics scraper
func NewMetricsScraper(configPath string) (*MetricsScraper, error) {
	// Create prometheus metrics
	statusGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "zstor_backend_status",
			Help: "Status of zstor backends (1 = alive, 0 = dead)",
		},
		[]string{"address", "backend_type", "namespace"},
	)

	lastScrapeTime := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "zstor_last_scrape_time",
			Help: "Timestamp of the last successful scrape",
		},
	)

	// Register metrics
	prometheus.MustRegister(statusGauge)
	prometheus.MustRegister(lastScrapeTime)

	scraper := &MetricsScraper{
		configPath:     configPath,
		backendStatus:  make(map[string]*BackendStatus),
		statusGauge:    statusGauge,
		lastScrapeTime: lastScrapeTime,
	}

	return scraper, nil
}

// GetPrometheusPort extracts the prometheus port from the zstor config file
func (ms *MetricsScraper) GetPrometheusPort() (int, error) {
	var config ZstorConfig
	if _, err := toml.DecodeFile(ms.configPath, &config); err != nil {
		return 0, fmt.Errorf("failed to parse zstor config: %w", err)
	}

	if config.PrometheusPort == 0 {
		return 9100, nil // Default prometheus port
	}

	return config.PrometheusPort, nil
}

// ScrapeMetrics fetches and processes metrics from the zstor prometheus endpoint
func (ms *MetricsScraper) ScrapeMetrics() error {
	port, err := ms.GetPrometheusPort()
	if err != nil {
		return fmt.Errorf("failed to get prometheus port: %w", err)
	}

	url := fmt.Sprintf("http://localhost:%d/metrics", port)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to fetch metrics from %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("received non-200 response from %s: %d", url, resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()

		// Look for connection_status metrics
		if strings.HasPrefix(line, "connection_status{") {
			ms.parseConnectionStatusMetric(line)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading metrics response: %w", err)
	}

	// Update prometheus metrics
	ms.updatePrometheusMetrics()

	// Update last scrape time
	ms.lastScrapeTime.Set(float64(time.Now().Unix()))

	return nil
}

// parseConnectionStatusMetric parses a connection_status metric line
func (ms *MetricsScraper) parseConnectionStatusMetric(line string) {
	// Example line:
	// connection_status{address="[45b:7cd9:4930:2763:3e54:4b4f:905b:dd16]:9900",backend_type="data",namespace="5545-1386679-qsfs_5545_data_921"} 1

	// Extract labels and value
	start := strings.Index(line, "{")
	end := strings.Index(line, "}")
	if start == -1 || end == -1 {
		return
	}

	labels := line[start+1 : end]
	valueStr := strings.TrimSpace(line[end+1:])

	// Parse value
	value, err := strconv.ParseFloat(valueStr, 64)
	if err != nil {
		return
	}

	// Parse labels
	labelMap := parseLabels(labels)

	// Create a unique key for this backend
	key := fmt.Sprintf("%s-%s-%s", labelMap["address"], labelMap["backend_type"], labelMap["namespace"])

	// Update or create backend status
	status, exists := ms.backendStatus[key]
	if !exists {
		status = &BackendStatus{
			Address:     labelMap["address"],
			BackendType: labelMap["backend_type"],
			Namespace:   labelMap["namespace"],
		}
		ms.backendStatus[key] = status
	}

	status.IsAlive = value == 1
	status.LastSeen = time.Now()
}

// parseLabels parses the labels from a prometheus metric line
func parseLabels(labels string) map[string]string {
	result := make(map[string]string)

	// Split by commas, but be careful of commas inside quoted strings
	parts := strings.Split(labels, "\",")
	for _, part := range parts {
		// Clean up the part
		part = strings.TrimSpace(part)
		if part[len(part)-1] == '"' {
			part = part[:len(part)-1]
		}

		// Split by equals
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 {
			key := strings.TrimSpace(kv[0])
			value := strings.TrimSpace(kv[1])

			// Remove quotes
			if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
				value = value[1 : len(value)-1]
			}

			result[key] = value
		}
	}

	return result
}

// updatePrometheusMetrics updates the prometheus metrics with current backend status
func (ms *MetricsScraper) updatePrometheusMetrics() {
	for _, status := range ms.backendStatus {
		value := 0.0
		if status.IsAlive {
			value = 1.0
		}

		ms.statusGauge.With(prometheus.Labels{
			"address":      status.Address,
			"backend_type": status.BackendType,
			"namespace":    status.Namespace,
		}).Set(value)
	}
}

// GetBackendLastSeen returns the last time a backend was seen alive
func (ms *MetricsScraper) GetBackendLastSeen(address, backendType, namespace string) (time.Time, bool) {
	key := fmt.Sprintf("%s-%s-%s", address, backendType, namespace)
	status, exists := ms.backendStatus[key]
	if !exists {
		return time.Time{}, false
	}

	return status.LastSeen, true
}

// GetBackendStatuses returns all current backend statuses
func (ms *MetricsScraper) GetBackendStatuses() map[string]*BackendStatus {
	return ms.backendStatus
}
