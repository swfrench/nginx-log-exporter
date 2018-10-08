package consumer

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/swfrench/nginx-log-exporter/exporter"
	"github.com/swfrench/nginx-log-exporter/tailer"
)

const (
	// ISO8601 contains a time.Parse reference timestamp for ISO 8601.
	ISO8601 = "2006-01-02T15:04:05-07:00"
)

type logLine struct {
	Time   string
	Status string
}

// Consumer implements periodic polling of the supplied nginx access log
// tailer, aggregation of response counts from the returned log lines, and
// reporting of the latter via the supplied exporter (e.g. to Stackdriver).
type Consumer struct {
	Period   time.Duration
	tailer   tailer.TailerT
	exporter exporter.ExporterT
	stop     chan bool
}

// NewConsumer returns a Consumer polling the supplied tailer and reporting to
// the supplied exporter with the specified period.
func NewConsumer(period time.Duration, tailer tailer.TailerT, exporter exporter.ExporterT) *Consumer {
	return &Consumer{
		Period:   period,
		tailer:   tailer,
		exporter: exporter,
		stop:     make(chan bool, 1),
	}
}

func (c *Consumer) consumeBytes(b []byte) error {
	statusCounts := make(map[string]float64)

	scanner := bufio.NewScanner(bytes.NewReader(b))
	for scanner.Scan() {
		lineBytes := scanner.Bytes()

		line := &logLine{}

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

		if t.After(c.exporter.StatusCounterCreationTime()) {
			if tot, ok := statusCounts[line.Status]; ok {
				statusCounts[line.Status] = 1 + tot
			} else {
				statusCounts[line.Status] = 1
			}
		}
	}

	return c.exporter.IncrementStatusCounter(statusCounts)
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
