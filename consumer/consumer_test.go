package consumer_test

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/swfrench/nginx-log-exporter/consumer"
	"github.com/swfrench/nginx-log-exporter/exporter"
)

type MockExporter struct {
	callCount            int
	detailedCallCount    int
	statusCounts         map[string]float64
	detailedStatusCounts map[string]exporter.DetailedStatusCount
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

type MockTailer struct {
	callCount int
	content   []byte
}

func (t *MockTailer) Next() ([]byte, error) {
	t.callCount += 1
	return t.content, nil
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

func TestBasicStatusCount(t *testing.T) {
	const testPeriod = 10 * time.Millisecond

	creationTime := time.Now()

	tailer := &MockTailer{}
	e := &MockExporter{creationTime: creationTime}
	c := consumer.NewConsumer(testPeriod, tailer, e, []string{})

	timeEarly := creationTime.Add(-1 * time.Minute).Format(consumer.ISO8601)
	timeLate := creationTime.Add(time.Minute).Format(consumer.ISO8601)

	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("{\"time\": \"%s\", \"status\": \"200\", \"request\": \"GET / HTTP/1.1\"}\n", timeEarly))
	buffer.WriteString(fmt.Sprintf("{\"time\": \"%s\", \"status\": \"200\", \"request\": \"GET /foo HTTP/1.1\"}\n", timeLate))
	buffer.WriteString(fmt.Sprintf("{\"time\": \"%s\", \"status\": \"200\", \"request\": \"POST /foo HTTP/1.1\"}\n", timeLate))
	buffer.WriteString(fmt.Sprintf("{\"time\": \"%s\", \"status\": \"500\", \"request\": \"GET /foo?bar=1 HTTP/1.1\"}\n", timeLate))

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
}

func TestDetailedStatusCount(t *testing.T) {
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
	buffer.WriteString(fmt.Sprintf("{\"time\": \"%s\", \"status\": \"200\", \"request\": \"GET / HTTP/1.1\"}\n", timeEarly))
	buffer.WriteString(fmt.Sprintf("{\"time\": \"%s\", \"status\": \"200\", \"request\": \"GET /foo HTTP/1.1\"}\n", timeEarly))
	buffer.WriteString(fmt.Sprintf("{\"time\": \"%s\", \"status\": \"200\", \"request\": \"GET / HTTP/1.1\"}\n", timeLate))
	buffer.WriteString(fmt.Sprintf("{\"time\": \"%s\", \"status\": \"200\", \"request\": \"GET /foo HTTP/1.1\"}\n", timeLate))
	buffer.WriteString(fmt.Sprintf("{\"time\": \"%s\", \"status\": \"500\", \"request\": \"GET /foo HTTP/1.1\"}\n", timeLate))
	buffer.WriteString(fmt.Sprintf("{\"time\": \"%s\", \"status\": \"200\", \"request\": \"POST /foo HTTP/1.1\"}\n", timeLate))
	buffer.WriteString(fmt.Sprintf("{\"time\": \"%s\", \"status\": \"500\", \"request\": \"GET /foo?bar=1 HTTP/1.1\"}\n", timeLate))

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

	// And content.
	if got, want := e.statusCounts["200"], float64(3); got != want {
		t.Fatalf("Exporter returned %v for 200 status count, wanted %v", got, want)
	}
	if got, want := e.statusCounts["500"], float64(2); got != want {
		t.Fatalf("Exporter returned %v for 500 status count, wanted %v", got, want)
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
