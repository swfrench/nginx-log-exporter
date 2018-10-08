package exporter_test

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/swfrench/nginx-log-exporter/exporter"
)

func TestStatusCounterCreationTime(t *testing.T) {
	tMin := time.Now()
	e := exporter.NewExporter(map[string]string{
		"foo": "bar",
	})
	tMax := time.Now()

	if creationTime := e.StatusCounterCreationTime(); creationTime.Before(tMin) || creationTime.After(tMax) {
		t.Fatalf("Reported counter creation time of %v is not in [%v, %v]", creationTime, tMin, tMax)
	}
	prometheus.Unregister(e.StatusCounterMetric())
}

func TestBasicStatusCounterUpdates(t *testing.T) {
	e := exporter.NewExporter(map[string]string{
		"foo": "bar",
		"bin": "baz",
	})

	e.IncrementStatusCounter(map[string]float64{
		"200": 2,
		"403": 1,
	})

	e.IncrementStatusCounter(map[string]float64{
		"500": 3,
	})

	// Expected metric counts and metadata.
	const expected = `
		# HELP http_response_count Counts of responses by status code
		# TYPE http_response_count counter
		http_response_count{foo="bar", bin="baz", status_code="200"} 2
		http_response_count{foo="bar", bin="baz", status_code="403"} 1
		http_response_count{foo="bar", bin="baz", status_code="500"} 3
	`

	if err := testutil.CollectAndCompare(e.StatusCounterMetric(), strings.NewReader(expected)); err != nil {
		t.Errorf("Collected metrics and / or metadata do not match expectation:\n%s", err)
	}
	prometheus.Unregister(e.StatusCounterMetric())
}
