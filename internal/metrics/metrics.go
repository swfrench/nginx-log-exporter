package metrics

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// CounterT is an interface for "wrapped" (i.e. owned by the Manager) counters.
type CounterT interface {
	Add(labels map[string]string, value float64) error
	Metric() *prometheus.CounterVec
	CreationTime() time.Time
}

// Counter is a concrete impl of CounterT.
type Counter struct {
	creationTime time.Time
	metric       *prometheus.CounterVec
}

// Metric returns a pointer to the underlying CounterVec.
func (c *Counter) Metric() *prometheus.CounterVec {
	return c.metric
}

// CreationTime returns the creation time of this metric.
func (c *Counter) CreationTime() time.Time {
	return c.creationTime
}

// Add adds the supplied value to the counter associated with the supplied
// labels.
func (c *Counter) Add(labels map[string]string, value float64) error {
	m, err := c.metric.GetMetricWith(labels)
	if err != nil {
		return err
	}
	m.Add(value)
	return nil
}

// HistogramT is an interface for "wrapped" (i.e. owned by the Manager)
// histograms.
type HistogramT interface {
	Observe(labels map[string]string, values []float64) error
	Metric() prometheus.ObserverVec
	CreationTime() time.Time
}

// Histogram is a concrete impl of HistogramT.
type Histogram struct {
	creationTime time.Time
	// Note: CurryWith returns an ObserverVec interface, rather than a
	// pointer.
	metric prometheus.ObserverVec
}

// Metric returns the underlying ObserverVec interface.
func (h *Histogram) Metric() prometheus.ObserverVec {
	return h.metric
}

// CreationTime returns the creation time of this metric.
func (h *Histogram) CreationTime() time.Time {
	return h.creationTime
}

// Observe records the slice of float64 observations in the histogram
// associated with the supplied labels.
func (h *Histogram) Observe(labels map[string]string, values []float64) error {
	m, err := h.metric.GetMetricWith(labels)
	if err != nil {
		return err
	}
	for _, value := range values {
		m.Observe(value)
	}
	return nil
}

// ManagerT is an interface representing a Manager (useful for mocks).
type ManagerT interface {
	AddCounter(name, help string, labelNames []string) error
	AddHistogram(name, help string, labelNames []string, buckets []float64) error
	GetCounter(name string) (CounterT, error)
	GetHistogram(name string) (HistogramT, error)
}

// Manager is an abstraction for ownership and access to counter and histogram
// metrics, intended to reduce boilerplate over managing Prometheus metrics
// directly.
type Manager struct {
	commonLabels map[string]string
	counters     map[string]*Counter
	histograms   map[string]*Histogram
}

// NewManager returns a Manager configured with the supplied "base" labels. All
// metrics created by the manager will be curried so as to already have those
// labels partially applied.
func NewManager(commonLabels map[string]string) *Manager {
	m := &Manager{
		counters:     make(map[string]*Counter),
		histograms:   make(map[string]*Histogram),
		commonLabels: make(map[string]string),
	}
	for k, v := range commonLabels {
		m.commonLabels[k] = v
	}
	return m
}

// AddCounter adds a counter metric with the supplied name, help string, and
// field labels.
func (m *Manager) AddCounter(name, help string, labelNames []string) error {
	var allLabels sort.StringSlice
	for k := range m.commonLabels {
		allLabels = append(allLabels, k)
	}
	allLabels = append(allLabels, labelNames...)
	allLabels.Sort()

	metric := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: name,
			Help: help,
		},
		allLabels,
	)
	if err := prometheus.Register(metric); err != nil {
		return err
	}

	partialMetric, err := metric.CurryWith(m.commonLabels)
	if err != nil {
		return err
	}

	m.counters[name] = &Counter{
		creationTime: time.Now(),
		metric:       partialMetric,
	}
	return nil
}

// AddHistogram adds a histogram metric with the supplied name, help string,
// field labels, and (optionally) buckets. Pass nil for buckets to use the
// defaults.
func (m *Manager) AddHistogram(name, help string, labelNames []string, buckets []float64) error {
	var allLabels sort.StringSlice
	for k := range m.commonLabels {
		allLabels = append(allLabels, k)
	}
	allLabels = append(allLabels, labelNames...)
	allLabels.Sort()

	opts := prometheus.HistogramOpts{
		Name: name,
		Help: help,
	}
	if buckets != nil {
		opts.Buckets = buckets
	}

	metric := prometheus.NewHistogramVec(opts, allLabels)
	if err := prometheus.Register(metric); err != nil {
		return err
	}

	partialMetric, err := metric.CurryWith(m.commonLabels)
	if err != nil {
		return err
	}

	m.histograms[name] = &Histogram{
		creationTime: time.Now(),
		metric:       partialMetric,
	}
	return nil
}

// GetCounter returns the counter with the specified name (i.e. passed on an
// earlier call to AddCounter). Note that the returned counter will already
// have the base labels supplied to the Manager partially applied.
func (m *Manager) GetCounter(name string) (CounterT, error) {
	c, ok := m.counters[name]
	if !ok {
		return nil, fmt.Errorf("unknown counter metric: %s", name)
	}
	return c, nil
}

// GetHistogram returns the histogram with the specified name (i.e. passed on
// an earlier call to AddHistogram). Note that the returned histogram will
// already have the base labels supplied to the Manager partially applied.
func (m *Manager) GetHistogram(name string) (HistogramT, error) {
	h, ok := m.histograms[name]
	if !ok {
		return nil, fmt.Errorf("unknown histogram metric: %s", name)
	}
	return h, nil
}

// UnregisterAll unregisters all previously created metrics from prometheus.
func (m *Manager) UnregisterAll() error {
	var failed []string
	for n, c := range m.counters {
		if !prometheus.Unregister(c.metric) {
			failed = append(failed, n)
		}
	}
	for n, h := range m.histograms {
		if !prometheus.Unregister(h.metric) {
			failed = append(failed, n)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("could not unregister: %s", strings.Join(failed, ", "))
	}
	return nil
}
