package exporter

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type ExporterT interface {
	IncrementStatusCounter(map[string]float64) error
	StatusCounterCreationTime() time.Time
}

type Exporter struct {
	statusCounter *prometheus.CounterVec
	creationTime  time.Time
	labelValues   []string
}

func NewExporter(labels map[string]string) *Exporter {
	e := &Exporter{}
	e.creationTime = time.Now()

	var labelKeys []string
	for key, value := range labels {
		labelKeys = append(labelKeys, key)
		e.labelValues = append(e.labelValues, value)
	}
	labelKeys = append(labelKeys, "status_code")

	e.statusCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_response_count",
			Help: "Counts of responses by status code",
		},
		labelKeys,
	)

	prometheus.MustRegister(e.statusCounter)

	return e
}

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

func (e *Exporter) StatusCounterCreationTime() time.Time {
	return e.creationTime
}

// StatusCounterMetric returns the underlying CounterVec used to track response codes. Only used in unit tests.
func (e *Exporter) StatusCounterMetric() *prometheus.CounterVec {
	return e.statusCounter
}
