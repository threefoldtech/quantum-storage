package zstor

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
)

// BackendStatus represents the status of a zstor backend
type BackendStatus struct {
	Address     string
	BackendType string
	Namespace   string
	IsAlive     bool
	LastSeen    time.Time
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
	config, err := LoadConfig(ms.configPath)
	if err != nil {
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
	log.Printf("Scraping metrics from %s", url)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to fetch metrics from %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("received non-200 response from %s: %d", url, resp.StatusCode)
	}

	// Parse metrics using expfmt
	parser := expfmt.TextParser{}
	metricFamilies, err := parser.TextToMetricFamilies(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to parse metrics: %w", err)
	}

	metricCount := 0
	// Process connection_status metrics
	if family, exists := metricFamilies["connection_status"]; exists {
		for _, metric := range family.Metric {
			ms.processConnectionStatusMetric(metric)
			metricCount++
		}
	}

	log.Printf("Found %d connection_status metrics", metricCount)

	// Update prometheus metrics
	ms.updatePrometheusMetrics()

	// Update last scrape time
	ms.lastScrapeTime.Set(float64(time.Now().Unix()))

	return nil
}

// processConnectionStatusMetric processes a connection_status metric
func (ms *MetricsScraper) processConnectionStatusMetric(metric *dto.Metric) {
	var address, backendType, namespace string

	// Extract labels
	for _, label := range metric.Label {
		switch label.GetName() {
		case "address":
			address = label.GetValue()
		case "backend_type":
			backendType = label.GetValue()
		case "namespace":
			namespace = label.GetValue()
		}
	}

	log.Printf("Processing connection status metric: address=%s, backend_type=%s, namespace=%s",
		address, backendType, namespace)

	// Extract value
	var value float64
	if metric.Gauge != nil {
		value = metric.Gauge.GetValue()
	} else if metric.Counter != nil {
		value = metric.Counter.GetValue()
	} else if metric.Untyped != nil {
		value = metric.Untyped.GetValue()
	}

	// Create a unique key for this backend
	key := fmt.Sprintf("%s-%s-%s", address, backendType, namespace)

	// Update or create backend status
	status, exists := ms.backendStatus[key]
	if !exists {
		status = &BackendStatus{
			Address:     address,
			BackendType: backendType,
			Namespace:   namespace,
		}
		ms.backendStatus[key] = status
	}

	status.IsAlive = value == 1
	status.LastSeen = time.Now()
}

// updatePrometheusMetrics updates the prometheus metrics with current backend status
func (ms *MetricsScraper) updatePrometheusMetrics() {
	log.Printf("Updating prometheus metrics with %d backend statuses", len(ms.backendStatus))
	for key, status := range ms.backendStatus {
		value := 0.0
		if status.IsAlive {
			value = 1.0
		}

		log.Printf("Backend status - key: %s, address: %s, type: %s, namespace: %s, alive: %t",
			key, status.Address, status.BackendType, status.Namespace, status.IsAlive)

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
	log.Printf("Returning %d backend statuses", len(ms.backendStatus))
	return ms.backendStatus
}
