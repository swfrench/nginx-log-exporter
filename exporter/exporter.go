package exporter

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type DetailedStatusCount struct {
	Count  float64
	Method string
	Path   string
	Status string
}

type ExporterT interface {
	IncrementStatusCounter(map[string]float64) error
	IncrementDetailedStatusCounter(map[string]DetailedStatusCount) error
	CreationTime() time.Time
}

type Exporter struct {
	statusCounter         *prometheus.CounterVec
	detailedStatusCounter *prometheus.CounterVec
	creationTime          time.Time
	labelValues           []string
}

// NewExporter returns a new Exporter with all metrics initialized.
func NewExporter(labels map[string]string) *Exporter {
	e := &Exporter{}
	e.creationTime = time.Now()

	var baseLabelKeys []string
	for key, value := range labels {
		baseLabelKeys = append(baseLabelKeys, key)
		e.labelValues = append(e.labelValues, value)
	}
	baseLabelKeys = append(baseLabelKeys, "status_code")

	e.statusCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_response_count",
			Help: "Counts of responses by status code",
		},
		baseLabelKeys,
	)

	prometheus.MustRegister(e.statusCounter)

	detailedLabelKeys := make([]string, len(baseLabelKeys))
	copy(detailedLabelKeys, baseLabelKeys)
	detailedLabelKeys = append(detailedLabelKeys, "path")
	detailedLabelKeys = append(detailedLabelKeys, "method")

	e.detailedStatusCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "detailed_http_response_count",
			Help: "Counts of responses by status code, path, and method",
		},
		detailedLabelKeys,
	)

	prometheus.MustRegister(e.detailedStatusCounter)

	return e
}

// IncrementStatusCounter increments the response count by status code metric.
// Single argument is a map from status (string) => increment.
func (e *Exporter) IncrementStatusCounter(counts map[string]float64) error {
	labels := make([]string, len(e.labelValues)+1)
	copy(labels, e.labelValues)

	for status, value := range counts {
		labels[len(e.labelValues)] = status
		m, err := e.statusCounter.GetMetricWithLabelValues(labels...)
		if err != nil {
			return err
		}
		m.Add(value)
	}

	return nil
}

// IncrementDetailedStatusCounter increments the detailed (i.e. annotated with
// path and method) variant of the status-code only response count metric.
// Single argument is a map from arbitrary key (string) => DetailedStatusCount
// struct (containing status, path, method, and increment). The key is
// arbitrary to allow clients to select their own unique addressing scheme.
func (e *Exporter) IncrementDetailedStatusCounter(counts map[string]DetailedStatusCount) error {
	labels := make([]string, len(e.labelValues)+3)
	copy(labels, e.labelValues)

	for _, value := range counts {
		labels[len(e.labelValues)] = value.Status
		labels[len(e.labelValues)+1] = value.Path
		labels[len(e.labelValues)+2] = value.Method
		m, err := e.detailedStatusCounter.GetMetricWithLabelValues(labels...)
		if err != nil {
			return err
		}
		m.Add(value.Count)
	}

	return nil
}

// CreationTime returns the time.Time when (or shortly before) the counter
// metrics were created.
func (e *Exporter) CreationTime() time.Time {
	return e.creationTime
}

// StatusCounterMetric returns the underlying CounterVec used to track response
// codes. Only used in unit tests.
func (e *Exporter) StatusCounterMetric() *prometheus.CounterVec {
	return e.statusCounter
}

// DetailedStatusCounterMetric returns the underlying CounterVec used to track
// response codes in detail. Only used in unit tests.
func (e *Exporter) DetailedStatusCounterMetric() *prometheus.CounterVec {
	return e.detailedStatusCounter
}

// Unregister unregisters all metrics. Only used in unit tests.
func (e *Exporter) Unregister() bool {
	success := true
	if !prometheus.Unregister(e.statusCounter) {
		success = false
	}
	if !prometheus.Unregister(e.detailedStatusCounter) {
		success = false
	}
	return success
}
