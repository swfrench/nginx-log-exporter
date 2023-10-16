// Tests for the generic metrics management logic in the metrics package.
//
// TODO(swfrench): Using testutil and matching exported metric literals is
// error prone. Should probably just mock out the metric types (and provide
// mock builders, rather than calling NewCounterVec etc. in exporter).

package metrics_test

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/swfrench/nginx-log-exporter/internal/metrics"
)

func TestCounterUpdates(t *testing.T) {
	m := metrics.NewManager(map[string]string{
		"foo": "bar",
	})

	tMin := time.Now()
	if err := m.AddCounter("foo_counter", "It counts things.", []string{
		"label_one",
		"label_two",
	}); err != nil {
		t.Fatalf("Counter creation failed: %v", err)
	}
	tMax := time.Now()

	c, err := m.GetCounter("foo_counter")
	if err != nil {
		t.Fatalf("Could not access newly created counter: %v", err)
	}

	if creationTime := c.CreationTime(); creationTime.Before(tMin) || creationTime.After(tMax) {
		t.Fatalf("Reported counter creation time of %v is not in [%v, %v]", creationTime, tMin, tMax)
	}

	for _, event := range []struct {
		labels    map[string]string
		increment float64
	}{
		{
			labels: map[string]string{
				"label_one": "one",
				"label_two": "two",
			},
			increment: 1,
		},
		{
			labels: map[string]string{
				"label_one": "one",
				"label_two": "two",
			},
			increment: 2,
		},
		{
			labels: map[string]string{
				"label_one": "three",
				"label_two": "four",
			},
			increment: 42,
		},
	} {
		if err := c.Add(event.labels, event.increment); err != nil {
			t.Fatalf("Failed to update counter: %v", err)
		}
	}

	const expected = `
		# HELP foo_counter It counts things.
		# TYPE foo_counter counter
		foo_counter{foo="bar",label_one="one",label_two="two"} 3.0
		foo_counter{foo="bar",label_one="three",label_two="four"} 42.0
	`

	if err := testutil.CollectAndCompare(c.Metric(), strings.NewReader(expected)); err != nil {
		t.Errorf("Collected metrics and / or metadata do not match expectation:\n%s", err)
	}
	if err := m.UnregisterAll(); err != nil {
		t.Fatalf("Failed to unregister one or more exported metrics: %v", err)
	}
}

func TestHistogramUpdates(t *testing.T) {
	m := metrics.NewManager(map[string]string{
		"foo": "bar",
	})

	tMin := time.Now()
	if err := m.AddHistogram("foo_dist", "It counts things, but in buckets.", []string{
		"label_one",
		"label_two",
	}, nil); err != nil {
		t.Fatalf("Histogram creation failed: %v", err)
	}
	tMax := time.Now()

	h, err := m.GetHistogram("foo_dist")
	if err != nil {
		t.Fatalf("Could not access newly created histogram: %v", err)
	}

	if creationTime := h.CreationTime(); creationTime.Before(tMin) || creationTime.After(tMax) {
		t.Fatalf("Reported histogram creation time of %v is not in [%v, %v]", creationTime, tMin, tMax)
	}

	for _, event := range []struct {
		labels       map[string]string
		observations []float64
	}{
		{
			labels: map[string]string{
				"label_one": "one",
				"label_two": "two",
			},
			observations: []float64{1},
		},
		{
			labels: map[string]string{
				"label_one": "one",
				"label_two": "two",
			},
			observations: []float64{1, 2},
		},
		{
			labels: map[string]string{
				"label_one": "three",
				"label_two": "four",
			},
			observations: []float64{3, 4},
		},
	} {
		if err := h.Observe(event.labels, event.observations); err != nil {
			t.Fatalf("Failed to update histogram: %v", err)
		}
	}

	const expected = `
		# HELP foo_dist It counts things, but in buckets.
		# TYPE foo_dist histogram
		foo_dist_bucket{foo="bar",label_one="one",label_two="two",le="0.005"} 0.0
		foo_dist_bucket{foo="bar",label_one="one",label_two="two",le="0.01"} 0.0
		foo_dist_bucket{foo="bar",label_one="one",label_two="two",le="0.025"} 0.0
		foo_dist_bucket{foo="bar",label_one="one",label_two="two",le="0.05"} 0.0
		foo_dist_bucket{foo="bar",label_one="one",label_two="two",le="0.1"} 0.0
		foo_dist_bucket{foo="bar",label_one="one",label_two="two",le="0.25"} 0.0
		foo_dist_bucket{foo="bar",label_one="one",label_two="two",le="0.5"} 0.0
		foo_dist_bucket{foo="bar",label_one="one",label_two="two",le="1.0"} 2.0
		foo_dist_bucket{foo="bar",label_one="one",label_two="two",le="2.5"} 3.0
		foo_dist_bucket{foo="bar",label_one="one",label_two="two",le="5.0"} 3.0
		foo_dist_bucket{foo="bar",label_one="one",label_two="two",le="10.0"} 3.0
		foo_dist_bucket{foo="bar",label_one="one",label_two="two",le="+Inf"} 3.0
		foo_dist_sum{foo="bar",label_one="one",label_two="two"} 4.0
		foo_dist_count{foo="bar",label_one="one",label_two="two"} 3.0
		foo_dist_bucket{foo="bar",label_one="three",label_two="four",le="0.005"} 0.0
		foo_dist_bucket{foo="bar",label_one="three",label_two="four",le="0.01"} 0.0
		foo_dist_bucket{foo="bar",label_one="three",label_two="four",le="0.025"} 0.0
		foo_dist_bucket{foo="bar",label_one="three",label_two="four",le="0.05"} 0.0
		foo_dist_bucket{foo="bar",label_one="three",label_two="four",le="0.1"} 0.0
		foo_dist_bucket{foo="bar",label_one="three",label_two="four",le="0.25"} 0.0
		foo_dist_bucket{foo="bar",label_one="three",label_two="four",le="0.5"} 0.0
		foo_dist_bucket{foo="bar",label_one="three",label_two="four",le="1.0"} 0.0
		foo_dist_bucket{foo="bar",label_one="three",label_two="four",le="2.5"} 0.0
		foo_dist_bucket{foo="bar",label_one="three",label_two="four",le="5.0"} 2.0
		foo_dist_bucket{foo="bar",label_one="three",label_two="four",le="10.0"} 2.0
		foo_dist_bucket{foo="bar",label_one="three",label_two="four",le="+Inf"} 2.0
		foo_dist_sum{foo="bar",label_one="three",label_two="four"} 7.0
		foo_dist_count{foo="bar",label_one="three",label_two="four"} 2.0
	`

	if err := testutil.CollectAndCompare(h.Metric(), strings.NewReader(expected)); err != nil {
		t.Errorf("Collected metrics and / or metadata do not match expectation:\n%s", err)
	}
	if err := m.UnregisterAll(); err != nil {
		t.Fatalf("Failed to unregister one or more exported metrics: %v", err)
	}
}

func TestHistogramUpdatesWithCustomBuckets(t *testing.T) {
	m := metrics.NewManager(map[string]string{
		"foo": "bar",
	})

	if err := m.AddHistogram("foo_dist", "It counts things, but in buckets.", []string{
		"label_one",
		"label_two",
	}, []float64{1, 2, 4, 8, 16}); err != nil {
		t.Fatalf("Histogram creation failed: %v", err)
	}

	h, err := m.GetHistogram("foo_dist")
	if err != nil {
		t.Fatalf("Could not access newly created histogram: %v", err)
	}

	for _, event := range []struct {
		labels       map[string]string
		observations []float64
	}{
		{
			labels: map[string]string{
				"label_one": "one",
				"label_two": "two",
			},
			observations: []float64{1},
		},
		{
			labels: map[string]string{
				"label_one": "one",
				"label_two": "two",
			},
			observations: []float64{1, 2},
		},
		{
			labels: map[string]string{
				"label_one": "three",
				"label_two": "four",
			},
			observations: []float64{3, 4},
		},
	} {
		if err := h.Observe(event.labels, event.observations); err != nil {
			t.Fatalf("Failed to update histogram: %v", err)
		}
	}

	const expected = `
		# HELP foo_dist It counts things, but in buckets.
		# TYPE foo_dist histogram
		foo_dist_bucket{foo="bar",label_one="one",label_two="two",le="1.0"} 2.0
		foo_dist_bucket{foo="bar",label_one="one",label_two="two",le="2.0"} 3.0
		foo_dist_bucket{foo="bar",label_one="one",label_two="two",le="4.0"} 3.0
		foo_dist_bucket{foo="bar",label_one="one",label_two="two",le="8.0"} 3.0
		foo_dist_bucket{foo="bar",label_one="one",label_two="two",le="16.0"} 3.0
		foo_dist_bucket{foo="bar",label_one="one",label_two="two",le="+Inf"} 3.0
		foo_dist_sum{foo="bar",label_one="one",label_two="two"} 4.0
		foo_dist_count{foo="bar",label_one="one",label_two="two"} 3.0
		foo_dist_bucket{foo="bar",label_one="three",label_two="four",le="1.0"} 0.0
		foo_dist_bucket{foo="bar",label_one="three",label_two="four",le="2.0"} 0.0
		foo_dist_bucket{foo="bar",label_one="three",label_two="four",le="4.0"} 2.0
		foo_dist_bucket{foo="bar",label_one="three",label_two="four",le="8.0"} 2.0
		foo_dist_bucket{foo="bar",label_one="three",label_two="four",le="16.0"} 2.0
		foo_dist_bucket{foo="bar",label_one="three",label_two="four",le="+Inf"} 2.0
		foo_dist_sum{foo="bar",label_one="three",label_two="four"} 7.0
		foo_dist_count{foo="bar",label_one="three",label_two="four"} 2.0
	`

	if err := testutil.CollectAndCompare(h.Metric(), strings.NewReader(expected)); err != nil {
		t.Errorf("Collected metrics and / or metadata do not match expectation:\n%s", err)
	}
	if err := m.UnregisterAll(); err != nil {
		t.Fatalf("Failed to unregister one or more exported metrics: %v", err)
	}
}
