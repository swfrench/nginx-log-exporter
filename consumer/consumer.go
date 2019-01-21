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

	"github.com/swfrench/nginx-log-exporter/exporter"
	"github.com/swfrench/nginx-log-exporter/tailer"
)

const (
	// ISO8601 contains a time.Parse reference timestamp for ISO 8601.
	ISO8601 = "2006-01-02T15:04:05-07:00"
)

type logLine struct {
	Time        string  `json:"time"`
	Request     string  `json:"request"`
	Status      string  `json:"status"`
	RequestTime float64 `json:"request_time"`
}

type logStats struct {
	statusCounts         map[string]float64
	latencyObservations  map[string][]float64
	detailedStatusCounts map[string]exporter.DetailedStatusCount
}

// Consumer implements periodic polling of the supplied nginx access log
// tailer, aggregation of response counts from the returned log lines, and
// reporting of the latter via the supplied exporter (e.g. to Stackdriver).
type Consumer struct {
	Period   time.Duration
	tailer   tailer.TailerT
	exporter exporter.ExporterT
	paths    map[string]bool
	stop     chan bool
}

// NewConsumer returns a Consumer polling the supplied tailer and reporting to
// the supplied exporter with the specified period.
func NewConsumer(period time.Duration, tailer tailer.TailerT, exporter exporter.ExporterT, paths []string) *Consumer {
	c := &Consumer{
		Period:   period,
		tailer:   tailer,
		exporter: exporter,
		paths:    make(map[string]bool),
		stop:     make(chan bool, 1),
	}
	for _, path := range paths {
		c.paths[path] = true
	}
	return c
}

func (c *Consumer) consumeLine(line *logLine, stats *logStats) {
	if tot, ok := stats.statusCounts[line.Status]; ok {
		stats.statusCounts[line.Status] = 1 + tot
	} else {
		stats.statusCounts[line.Status] = 1
	}

	if line.RequestTime >= 0 {
		stats.latencyObservations[line.Status] = append(stats.latencyObservations[line.Status], line.RequestTime)
	}

	if requestFields := strings.Fields(line.Request); len(requestFields) != 3 {
		log.Printf("Skipping malformed request field: %v", line.Request)
	} else if u, err := url.ParseRequestURI(requestFields[1]); err != nil {
		log.Printf("Skipping malformed request path: %v", requestFields[1])
	} else if _, ok := c.paths[u.Path]; ok {
		key := strings.Join([]string{line.Status, requestFields[0], u.Path}, ":")
		if tot, ok := stats.detailedStatusCounts[key]; ok {
			tot.Count += 1
			stats.detailedStatusCounts[key] = tot
		} else {
			stats.detailedStatusCounts[key] = exporter.DetailedStatusCount{
				Count:  1,
				Status: line.Status,
				Path:   u.Path,
				Method: requestFields[0],
			}
		}
	}
}

func (c *Consumer) consumeBytes(b []byte) error {
	stats := &logStats{
		statusCounts:         make(map[string]float64),
		latencyObservations:  make(map[string][]float64),
		detailedStatusCounts: make(map[string]exporter.DetailedStatusCount),
	}

	scanner := bufio.NewScanner(bytes.NewReader(b))
	for scanner.Scan() {
		lineBytes := scanner.Bytes()

		line := &logLine{
			// Sentinal value in case request time was not present.
			RequestTime: -1,
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

		if t.After(c.exporter.CreationTime()) {
			c.consumeLine(line, stats)
		}
	}

	if err := c.exporter.IncrementStatusCounter(stats.statusCounts); err != nil {
		return fmt.Errorf("Call to IncrementStatusCounter failed: %v", err)
	}
	if err := c.exporter.IncrementDetailedStatusCounter(stats.detailedStatusCounts); err != nil {
		return fmt.Errorf("Call to IncrementDetailedStatusCounter failed: %v", err)
	}
	if err := c.exporter.RecordLatencyObservations(stats.latencyObservations); err != nil {
		return fmt.Errorf("Call to RecordLatencyObservations failed: %v", err)
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
