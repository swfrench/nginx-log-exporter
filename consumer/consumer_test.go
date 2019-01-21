package consumer_test

import (
	"bytes"
	"math"
	"sort"
	"testing"
	"text/template"
	"time"

	"github.com/swfrench/nginx-log-exporter/consumer"
	"github.com/swfrench/nginx-log-exporter/exporter"
)

var (
	logTemplate *template.Template
)

const (
	floatEqualityAbsoluteTol = 1e-9
	logTemplateFormat        = "{\"time\": \"{{.Time}}\", \"status\": \"{{.Status}}\", \"request_time\": {{.RequestTime}}, \"request\": \"{{.Method}} {{.Path}} HTTP/1.1\"}\n"
)

func init() {
	logTemplate = template.Must(template.New("logLine").Parse(logTemplateFormat))
}

// Mocks:

type MockExporter struct {
	callCount            int
	detailedCallCount    int
	latencyCallCount     int
	statusCounts         map[string]float64
	detailedStatusCounts map[string]exporter.DetailedStatusCount
	latencyObservations  map[string][]float64
	creationTime         time.Time
}

func (e *MockExporter) CreationTime() time.Time {
	return e.creationTime
}

func (e *MockExporter) IncrementStatusCounter(counts map[string]float64) error {
	e.callCount += 1
	e.statusCounts = make(map[string]float64)
	for code := range counts {
		e.statusCounts[code] = counts[code]
	}
	return nil
}

func (e *MockExporter) IncrementDetailedStatusCounter(counts map[string]exporter.DetailedStatusCount) error {
	e.detailedCallCount += 1
	e.detailedStatusCounts = make(map[string]exporter.DetailedStatusCount)
	for code := range counts {
		e.detailedStatusCounts[code] = counts[code]
	}
	return nil
}

func (e *MockExporter) RecordLatencyObservations(obs map[string][]float64) error {
	e.latencyCallCount += 1
	e.latencyObservations = make(map[string][]float64)
	for code := range obs {
		e.latencyObservations[code] = append(e.latencyObservations[code], obs[code]...)
	}
	return nil
}

type MockTailer struct {
	callCount int
	content   []byte
}

func (t *MockTailer) Next() ([]byte, error) {
	t.callCount += 1
	return t.content, nil
}

// Helpers:

type logLine struct {
	Time        string
	Status      string
	RequestTime string
	Method      string
	Path        string
}

func buildLogLine(line logLine, buffer *bytes.Buffer) error {
	return logTemplate.Execute(buffer, line)
}

func floatEq(a, b float64) bool {
	// TODO(swfrench): Maybe relative equality.
	return math.Abs(a-b) <= floatEqualityAbsoluteTol
}

func unorderedFloatElementsEq(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	as := make(sort.Float64Slice, len(a))
	bs := make(sort.Float64Slice, len(b))
	copy(as, a)
	copy(bs, b)
	// NOTE(swfrench): Assumption: comparison under Sort operates within floatEq tolerance.
	sort.Sort(as)
	sort.Sort(bs)
	for i := range as {
		if !floatEq(as[i], bs[i]) {
			return false
		}
	}
	return true
}

func testRunConsumer(t *testing.T, c *consumer.Consumer) {
	done := make(chan bool, 1)
	var consumerErr error
	go func() {
		consumerErr = c.Run()
		done <- true
	}()

	// Wait at least two polling periods and stop.
	time.Sleep(2 * c.Period)
	c.Stop()

	// Ensure the consumer terminated in a timely manner.
	time.Sleep(c.Period)
	select {
	case <-done:
	default:
		t.Fatalf("Consumer did not terminate after calling Stop()")
	}

	// Check for errors emitted by the consumer.
	if consumerErr != nil {
		t.Fatalf("Consumer returned with error: %v", consumerErr)
	}
}

// Tests:

func TestSimple(t *testing.T) {
	const testPeriod = 10 * time.Millisecond

	tailer := &MockTailer{}
	e := &MockExporter{}
	c := consumer.NewConsumer(testPeriod, tailer, e, []string{})

	testRunConsumer(t, c)

	// Now check call counts.
	if tailer.callCount == 0 {
		t.Fatalf("Consumer did not call MockTailer.Next()")
	}
	if e.callCount == 0 {
		t.Fatalf("Consumer did not call MockExporter.IncrementStatusCounter()")
	}
}

func TestBasicStats(t *testing.T) {
	const testPeriod = 10 * time.Millisecond

	creationTime := time.Now()

	tailer := &MockTailer{}
	e := &MockExporter{creationTime: creationTime}
	c := consumer.NewConsumer(testPeriod, tailer, e, []string{})

	timeEarly := creationTime.Add(-1 * time.Minute).Format(consumer.ISO8601)
	timeLate := creationTime.Add(time.Minute).Format(consumer.ISO8601)

	var buffer bytes.Buffer
	for _, line := range []logLine{
		{
			Time:        timeEarly,
			Status:      "200",
			RequestTime: "0.010",
			Method:      "GET",
			Path:        "/",
		},
		{
			Time:        timeLate,
			Status:      "200",
			RequestTime: "0.020",
			Method:      "GET",
			Path:        "/foo",
		},
		{
			Time:        timeLate,
			Status:      "200",
			RequestTime: "0.030",
			Method:      "POST",
			Path:        "/foo",
		},
		{
			Time:        timeLate,
			Status:      "500",
			RequestTime: "0.040",
			Method:      "GET",
			Path:        "/foo?bar=1",
		},
	} {
		buildLogLine(line, &buffer)
	}

	tailer.content = buffer.Bytes()

	testRunConsumer(t, c)

	// Now check call counts.
	if tailer.callCount == 0 {
		t.Fatalf("Consumer did not call MockTailer.Next()")
	}
	if e.callCount == 0 {
		t.Fatalf("Consumer did not call MockExporter.IncrementStatusCounter()")
	}
	if e.detailedCallCount == 0 {
		t.Fatalf("Consumer did not call MockExporter.IncrementDetailedStatusCounter()")
	}
	if e.latencyCallCount == 0 {
		t.Fatalf("Consumer did not call MockExporter.RecordLatencyObservations()")
	}

	// And content.
	if got, want := e.statusCounts["200"], float64(2); got != want {
		t.Fatalf("Exporter returned %v for 200 status count, wanted %v", got, want)
	}
	if got, want := e.statusCounts["500"], float64(1); got != want {
		t.Fatalf("Exporter returned %v for 500 status count, wanted %v", got, want)
	}
	if got, want := len(e.detailedStatusCounts), 0; got != want {
		t.Fatalf("Exporter was provided with detailed status counts, even though no paths were configured; got: %v", e.detailedStatusCounts)
	}
	for code, obs := range map[string][]float64{
		"200": {0.02, 0.03},
		"500": {0.04},
	} {
		if got, want := e.latencyObservations[code], obs; !unorderedFloatElementsEq(got, want) {
			t.Fatalf("Exporter was provided with a set of latency observations that did not match for status %s: got %v, want %v", code, got, want)
		}
	}
}

func TestDetailedStats(t *testing.T) {
	const testPeriod = 10 * time.Millisecond

	creationTime := time.Now()

	tailer := &MockTailer{}
	e := &MockExporter{creationTime: creationTime}
	c := consumer.NewConsumer(testPeriod, tailer, e, []string{
		"/foo",
	})

	timeEarly := creationTime.Add(-1 * time.Minute).Format(consumer.ISO8601)
	timeLate := creationTime.Add(time.Minute).Format(consumer.ISO8601)

	var buffer bytes.Buffer
	for _, line := range []logLine{
		{
			Time:        timeEarly,
			Status:      "200",
			RequestTime: "0.010",
			Method:      "GET",
			Path:        "/",
		},
		{
			Time:        timeEarly,
			Status:      "200",
			RequestTime: "0.010",
			Method:      "GET",
			Path:        "/foo",
		},
		{
			Time:        timeLate,
			Status:      "200",
			RequestTime: "0.020",
			Method:      "GET",
			Path:        "/",
		},
		{
			Time:        timeLate,
			Status:      "200",
			RequestTime: "0.030",
			Method:      "GET",
			Path:        "/foo",
		},
		{
			Time:        timeLate,
			Status:      "500",
			RequestTime: "0.040",
			Method:      "GET",
			Path:        "/foo",
		},
		{
			Time:        timeLate,
			Status:      "200",
			RequestTime: "0.050",
			Method:      "POST",
			Path:        "/foo",
		},
		{
			Time:        timeLate,
			Status:      "500",
			RequestTime: "0.060",
			Method:      "GET",
			Path:        "/foo?bar=1",
		},
	} {
		buildLogLine(line, &buffer)
	}

	tailer.content = buffer.Bytes()

	testRunConsumer(t, c)

	// Now check call counts.
	if tailer.callCount == 0 {
		t.Fatalf("Consumer did not call MockTailer.Next()")
	}
	if e.callCount == 0 {
		t.Fatalf("Consumer did not call MockExporter.IncrementStatusCounter()")
	}
	if e.detailedCallCount == 0 {
		t.Fatalf("Consumer did not call MockExporter.IncrementDetailedStatusCounter()")
	}
	if e.latencyCallCount == 0 {
		t.Fatalf("Consumer did not call MockExporter.RecordLatencyObservations()")
	}

	// And content.
	if got, want := e.statusCounts["200"], float64(3); got != want {
		t.Fatalf("Exporter returned %v for 200 status count, wanted %v", got, want)
	}
	if got, want := e.statusCounts["500"], float64(2); got != want {
		t.Fatalf("Exporter returned %v for 500 status count, wanted %v", got, want)
	}
	for code, obs := range map[string][]float64{
		"200": {0.02, 0.03, 0.05},
		"500": {0.04, 0.06},
	} {
		if got, want := e.latencyObservations[code], obs; !unorderedFloatElementsEq(got, want) {
			t.Fatalf("Exporter was provided with a set of latency observations that did not match for status %s: got %v, want %v", code, got, want)
		}
	}
	for _, expected := range []exporter.DetailedStatusCount{
		{
			Count:  1,
			Path:   "/foo",
			Method: "GET",
			Status: "200",
		},
		{
			Count:  1,
			Path:   "/foo",
			Method: "POST",
			Status: "200",
		},
		{
			Count:  2,
			Path:   "/foo",
			Method: "GET",
			Status: "500",
		},
	} {
		found := false
		for _, got := range e.detailedStatusCounts {
			if got == expected {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("Expected exporter to be called with %v, but this is not in: %v", expected, e.detailedStatusCounts)
		}
	}
}
