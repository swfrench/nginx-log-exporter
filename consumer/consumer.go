package consumer

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/swfrench/nginx-log-exporter/metrics"
	"github.com/swfrench/nginx-log-exporter/tailer"
)

const (
	// ISO8601 contains a time.Parse reference timestamp for ISO 8601.
	ISO8601 = "2006-01-02T15:04:05-07:00"
)

var (
	// Buckets used with the http_response_bytes_sent metric.
	bytesSentBuckets = []float64{8, 16, 64, 128, 256, 512, 1024, 2048, 4096}
)

type logLine struct {
	Time        string  `json:"time"`
	Request     string  `json:"request"`
	Status      string  `json:"status"`
	RequestTime float64 `json:"request_time"`
	BytesSent   float64 `json:"bytes_sent"`
}

type annotatedCount struct {
	total       float64
	annotations map[string]string
}

type keyedCounter struct {
	counts map[string]*annotatedCount
}

func newKeyedCounter() *keyedCounter {
	return &keyedCounter{
		counts: make(map[string]*annotatedCount),
	}
}

func (c *keyedCounter) inc(key string, annotations map[string]string) {
	if _, ok := c.counts[key]; ok {
		c.counts[key].total += 1
		return
	}

	a := &annotatedCount{
		total:       1,
		annotations: nil,
	}
	if annotations != nil {
		a.annotations = make(map[string]string)
		for k, v := range annotations {
			a.annotations[k] = v
		}
	}
	c.counts[key] = a
}

type annotatedObservations struct {
	seen        []float64
	annotations map[string]string
}

type keyedAccumulator struct {
	observations map[string]*annotatedObservations
}

func newKeyedAccumulator() *keyedAccumulator {
	return &keyedAccumulator{
		observations: make(map[string]*annotatedObservations),
	}
}

func (a *keyedAccumulator) record(key string, value float64, annotations map[string]string) {
	if _, ok := a.observations[key]; ok {
		a.observations[key].seen = append(a.observations[key].seen, value)
		return
	}

	o := &annotatedObservations{
		annotations: nil,
	}
	o.seen = append(o.seen, value)
	if annotations != nil {
		o.annotations = make(map[string]string)
		for k, v := range annotations {
			o.annotations[k] = v
		}
	}
	a.observations[key] = o
}

type logStats struct {
	statusCounts          *keyedCounter
	detailedStatusCounts  *keyedCounter
	latencyObservations   *keyedAccumulator
	bytesSentObservations *keyedAccumulator
}

// Consumer implements periodic polling of the supplied nginx access log
// tailer, aggregation of response counts from the returned log lines.
type Consumer struct {
	Period                     time.Duration
	tailer                     tailer.TailerT
	manager                    metrics.MetricsManagerT
	paths                      map[string]bool
	stop                       chan bool
	initFinshed                time.Time
	httpResposeCounter         metrics.CounterT
	detailedHttpResposeCounter metrics.CounterT
	httpResposeTimeHist        metrics.HistogramT
	httpResposeByteSentHist    metrics.HistogramT
}

// NewConsumer returns a Consumer polling the supplied tailer for new access
// log lines and exporting counts / stats to the supplied manager at the
// specified period. The specific metrics exported by the Consumer will be
// created during init in NewConsumer.
func NewConsumer(period time.Duration, tailer tailer.TailerT, manager metrics.MetricsManagerT, paths []string) (*Consumer, error) {
	c := &Consumer{
		Period:  period,
		tailer:  tailer,
		manager: manager,
		paths:   make(map[string]bool),
		stop:    make(chan bool, 1),
	}
	for _, path := range paths {
		c.paths[path] = true
	}

	if err := manager.AddCounter("http_response_count", "Counts of responses by status code", []string{
		"status_code",
	}); err != nil {
		return nil, err
	}
	if counter, err := manager.GetCounter("http_response_count"); err != nil {
		return nil, err
	} else {
		c.httpResposeCounter = counter
	}

	if err := manager.AddCounter("detailed_http_response_count", "Counts of responses by status code, path, and method", []string{
		"status_code",
		"path",
		"method",
	}); err != nil {
		return nil, err
	}
	if counter, err := manager.GetCounter("detailed_http_response_count"); err != nil {
		return nil, err
	} else {
		c.detailedHttpResposeCounter = counter
	}

	if err := manager.AddHistogram("http_response_time", "Response time (seconds) by status code", []string{
		"status_code",
	}, nil); err != nil {
		return nil, err
	}
	if hist, err := manager.GetHistogram("http_response_time"); err != nil {
		return nil, err
	} else {
		c.httpResposeTimeHist = hist
	}

	if err := manager.AddHistogram("http_response_bytes_sent", "Response size (bytes) by status code", []string{
		"status_code",
	}, bytesSentBuckets); err != nil {
		return nil, err
	}
	if hist, err := manager.GetHistogram("http_response_bytes_sent"); err != nil {
		return nil, err
	} else {
		c.httpResposeByteSentHist = hist
	}

	c.initFinshed = time.Now()

	return c, nil
}

func (c *Consumer) consumeLine(line *logLine, stats *logStats) {
	stats.statusCounts.inc(line.Status, nil)

	if line.RequestTime >= 0 {
		stats.latencyObservations.record(line.Status, line.RequestTime, nil)
	}

	if line.BytesSent >= 0 {
		stats.bytesSentObservations.record(line.Status, line.BytesSent, nil)
	}

	if requestFields := strings.Fields(line.Request); len(requestFields) != 3 {
		log.Printf("Skipping malformed request field: %v", line.Request)
	} else if u, err := url.ParseRequestURI(requestFields[1]); err != nil {
		log.Printf("Skipping malformed request path: %v", requestFields[1])
	} else if _, ok := c.paths[u.Path]; ok {
		key := strings.Join([]string{line.Status, requestFields[0], u.Path}, ":")
		stats.detailedStatusCounts.inc(key, map[string]string{
			"status_code": line.Status,
			"path":        u.Path,
			"method":      requestFields[0],
		})
	}
}

func (c *Consumer) consumeBytes(b []byte) error {
	stats := &logStats{
		statusCounts:          newKeyedCounter(),
		detailedStatusCounts:  newKeyedCounter(),
		latencyObservations:   newKeyedAccumulator(),
		bytesSentObservations: newKeyedAccumulator(),
	}

	scanner := bufio.NewScanner(bytes.NewReader(b))
	for scanner.Scan() {
		lineBytes := scanner.Bytes()

		line := &logLine{
			// Sentinal values for numeric fields that might not be present.
			RequestTime: -1,
			BytesSent:   -1,
		}

		err := json.Unmarshal(lineBytes, line)
		if err != nil {
			log.Printf("Error parsing log line: %v", err)
			continue
		}

		t, err := time.Parse(ISO8601, line.Time)
		if err != nil {
			log.Printf("Could not parse time %v: %v", line.Time, err)
			continue
		}

		if t.After(c.initFinshed) {
			c.consumeLine(line, stats)
		}
	}

	for code, count := range stats.statusCounts.counts {
		if err := c.httpResposeCounter.Add(map[string]string{
			"status_code": code,
		}, count.total); err != nil {
			return err
		}
	}
	for code, count := range stats.detailedStatusCounts.counts {
		labels := map[string]string{
			"status_code": code,
		}
		if count.annotations != nil {
			for k, v := range count.annotations {
				labels[k] = v
			}
		}
		if err := c.detailedHttpResposeCounter.Add(labels, count.total); err != nil {
			return err
		}
	}
	for code, observations := range stats.latencyObservations.observations {
		if err := c.httpResposeTimeHist.Observe(map[string]string{
			"status_code": code,
		}, observations.seen); err != nil {
			return err
		}
	}
	for code, observations := range stats.bytesSentObservations.observations {
		if err := c.httpResposeByteSentHist.Observe(map[string]string{
			"status_code": code,
		}, observations.seen); err != nil {
			return err
		}
	}
	return nil
}

// Run performs periodic polling and exporting. It will only return on error or
// if Stop is called.
func (c *Consumer) Run() error {
	for {
		select {
		case <-time.After(c.Period):
		case <-c.stop:
			return nil
		}
		b, err := c.tailer.Next()
		if err != nil {
			return fmt.Errorf("Could not retrieve log content: %v", err)
		} else if err := c.consumeBytes(b); err != nil {
			return fmt.Errorf("Could not export log content: %v", err)
		}
	}
	return nil
}

// Stop signals that polling should cease in Run and the latter should return
// (e.g. if Run is blocking in another goroutine).
func (c *Consumer) Stop() {
	c.stop <- true
}
