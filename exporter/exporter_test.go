// TODO(swfrench): Using testutil and matching exported metric literals is
// error prone. Should probably just mock out the metric types (and provide
// mock builders, rather than calling NewCounterVec etc. in exporter).

package exporter_test

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/swfrench/nginx-log-exporter/exporter"
)

func TestStatusCounterCreationTime(t *testing.T) {
	tMin := time.Now()
	e := exporter.NewExporter(map[string]string{
		"foo": "bar",
	})
	tMax := time.Now()

	if creationTime := e.CreationTime(); creationTime.Before(tMin) || creationTime.After(tMax) {
		t.Fatalf("Reported counter creation time of %v is not in [%v, %v]", creationTime, tMin, tMax)
	}
	if !e.Unregister() {
		t.Fatalf("Failed to unregister one or more exported metrics.")
	}
}

func TestBasicStatusCounterUpdates(t *testing.T) {
	e := exporter.NewExporter(map[string]string{
		"foo": "bar",
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
		http_response_count{foo="bar", status_code="200"} 2
		http_response_count{foo="bar", status_code="403"} 1
		http_response_count{foo="bar", status_code="500"} 3
	`

	if err := testutil.CollectAndCompare(e.StatusCounterMetric(), strings.NewReader(expected)); err != nil {
		t.Errorf("Collected metrics and / or metadata do not match expectation:\n%s", err)
	}
	if !e.Unregister() {
		t.Fatalf("Failed to unregister one or more exported metrics.")
	}
}

func TestDetailedStatusCounterUpdates(t *testing.T) {
	e := exporter.NewExporter(map[string]string{
		"foo": "bar",
	})

	e.IncrementDetailedStatusCounter(map[string]exporter.DetailedStatusCount{
		"200:GET:/baz": exporter.DetailedStatusCount{
			Status: "200",
			Count:  2,
			Path:   "/baz",
			Method: "GET",
		},
		"403:POST:/boz": exporter.DetailedStatusCount{
			Status: "403",
			Count:  1,
			Path:   "/boz",
			Method: "POST",
		},
	})

	e.IncrementDetailedStatusCounter(map[string]exporter.DetailedStatusCount{
		"500:POST:/baz": exporter.DetailedStatusCount{
			Status: "500",
			Count:  3,
			Path:   "/baz",
			Method: "POST",
		},
	})

	// Expected metric counts and metadata.
	const expected = `
		# HELP detailed_http_response_count Counts of responses by status code, path, and method
		# TYPE detailed_http_response_count counter
		detailed_http_response_count{foo="bar", method="GET",  path="/baz", status_code="200"} 2
		detailed_http_response_count{foo="bar", method="POST", path="/baz", status_code="500"} 3
		detailed_http_response_count{foo="bar", method="POST", path="/boz", status_code="403"} 1
	`

	if err := testutil.CollectAndCompare(e.DetailedStatusCounterMetric(), strings.NewReader(expected)); err != nil {
		t.Errorf("Collected metrics and / or metadata do not match expectation:\n%s", err)
	}
	if !e.Unregister() {
		t.Fatalf("Failed to unregister one or more exported metrics.")
	}
}

func TestLatencyHistogramUpdates(t *testing.T) {
	e := exporter.NewExporter(map[string]string{
		"foo": "bar",
	})

	e.RecordLatencyObservations(map[string][]float64{
		"200": {0.01, 0.02},
		"403": {0.01},
	})

	e.RecordLatencyObservations(map[string][]float64{
		"200": {0.01},
		"500": {0.10},
	})

	const expected = `
		# HELP http_response_time Response time (seconds) by status code
		# TYPE http_response_time histogram
		http_response_time_bucket{foo="bar",status_code="200",le="0.005"} 0.0
		http_response_time_bucket{foo="bar",status_code="200",le="0.01"} 2.0
		http_response_time_bucket{foo="bar",status_code="200",le="0.025"} 3.0
		http_response_time_bucket{foo="bar",status_code="200",le="0.05"} 3.0
		http_response_time_bucket{foo="bar",status_code="200",le="0.1"} 3.0
		http_response_time_bucket{foo="bar",status_code="200",le="0.25"} 3.0
		http_response_time_bucket{foo="bar",status_code="200",le="0.5"} 3.0
		http_response_time_bucket{foo="bar",status_code="200",le="1.0"} 3.0
		http_response_time_bucket{foo="bar",status_code="200",le="2.5"} 3.0
		http_response_time_bucket{foo="bar",status_code="200",le="5.0"} 3.0
		http_response_time_bucket{foo="bar",status_code="200",le="10.0"} 3.0
		http_response_time_bucket{foo="bar",status_code="200",le="+Inf"} 3.0
		http_response_time_sum{foo="bar",status_code="200"} 0.04
		http_response_time_count{foo="bar",status_code="200"} 3.0
		http_response_time_bucket{foo="bar",status_code="403",le="0.005"} 0.0
		http_response_time_bucket{foo="bar",status_code="403",le="0.01"} 1.0
		http_response_time_bucket{foo="bar",status_code="403",le="0.025"} 1.0
		http_response_time_bucket{foo="bar",status_code="403",le="0.05"} 1.0
		http_response_time_bucket{foo="bar",status_code="403",le="0.1"} 1.0
		http_response_time_bucket{foo="bar",status_code="403",le="0.25"} 1.0
		http_response_time_bucket{foo="bar",status_code="403",le="0.5"} 1.0
		http_response_time_bucket{foo="bar",status_code="403",le="1.0"} 1.0
		http_response_time_bucket{foo="bar",status_code="403",le="2.5"} 1.0
		http_response_time_bucket{foo="bar",status_code="403",le="5.0"} 1.0
		http_response_time_bucket{foo="bar",status_code="403",le="10.0"} 1.0
		http_response_time_bucket{foo="bar",status_code="403",le="+Inf"} 1.0
		http_response_time_sum{foo="bar",status_code="403"} 0.01
		http_response_time_count{foo="bar",status_code="403"} 1.0
		http_response_time_bucket{foo="bar",status_code="500",le="0.005"} 0.0
		http_response_time_bucket{foo="bar",status_code="500",le="0.01"} 0.0
		http_response_time_bucket{foo="bar",status_code="500",le="0.025"} 0.0
		http_response_time_bucket{foo="bar",status_code="500",le="0.05"} 0.0
		http_response_time_bucket{foo="bar",status_code="500",le="0.1"} 1.0
		http_response_time_bucket{foo="bar",status_code="500",le="0.25"} 1.0
		http_response_time_bucket{foo="bar",status_code="500",le="0.5"} 1.0
		http_response_time_bucket{foo="bar",status_code="500",le="1.0"} 1.0
		http_response_time_bucket{foo="bar",status_code="500",le="2.5"} 1.0
		http_response_time_bucket{foo="bar",status_code="500",le="5.0"} 1.0
		http_response_time_bucket{foo="bar",status_code="500",le="10.0"} 1.0
		http_response_time_bucket{foo="bar",status_code="500",le="+Inf"} 1.0
		http_response_time_sum{foo="bar",status_code="500"} 0.1
		http_response_time_count{foo="bar",status_code="500"} 1.0
	`

	if err := testutil.CollectAndCompare(e.ResponseLatencyHistogramMetric(), strings.NewReader(expected)); err != nil {
		t.Errorf("Collected metrics and / or metadata do not match expectation:\n%s", err)
	}
	if !e.Unregister() {
		t.Fatalf("Failed to unregister one or more exported metrics.")
	}
}
