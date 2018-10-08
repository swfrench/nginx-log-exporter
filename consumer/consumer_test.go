package consumer_test

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/swfrench/nginx-log-exporter/consumer"
)

type MockExporter struct {
	callCount    int
	statusCounts map[string]float64
	creationTime time.Time
}

func (e *MockExporter) StatusCounterCreationTime() time.Time {
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
	exporter := &MockExporter{}
	c := consumer.NewConsumer(testPeriod, tailer, exporter)

	testRunConsumer(t, c)

	// Now check call counts.
	if tailer.callCount == 0 {
		t.Fatalf("Consumer did not call MockTailer.Next()")
	}
	if exporter.callCount == 0 {
		t.Fatalf("Consumer did not call MockExporter.IncrementStatusCounter()")
	}
}

func TestStatusCount(t *testing.T) {
	const testPeriod = 10 * time.Millisecond

	creationTime := time.Now()

	tailer := &MockTailer{}
	exporter := &MockExporter{creationTime: creationTime}
	c := consumer.NewConsumer(testPeriod, tailer, exporter)

	timeEarly := creationTime.Add(-1 * time.Minute).Format(consumer.ISO8601)
	timeLate := creationTime.Add(time.Minute).Format(consumer.ISO8601)

	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("{\"time\": \"%s\", \"status\": \"200\"}\n", timeEarly))
	buffer.WriteString(fmt.Sprintf("{\"time\": \"%s\", \"status\": \"200\"}\n", timeLate))
	buffer.WriteString(fmt.Sprintf("{\"time\": \"%s\", \"status\": \"200\"}\n", timeLate))
	buffer.WriteString(fmt.Sprintf("{\"time\": \"%s\", \"status\": \"500\"}\n", timeLate))

	tailer.content = buffer.Bytes()

	testRunConsumer(t, c)

	// Now check call counts.
	if tailer.callCount == 0 {
		t.Fatalf("Consumer did not call MockTailer.Next()")
	}
	if exporter.callCount == 0 {
		t.Fatalf("Consumer did not call MockExporter.StatusCounts()")
	}

	// And content.
	if got, want := exporter.statusCounts["200"], float64(2); got != want {
		t.Fatalf("Exporter returned %v for 200 status count, wanted %v", got, want)
	}
	if got, want := exporter.statusCounts["500"], float64(1); got != want {
		t.Fatalf("Exporter returned %v for 500 status count, wanted %v", got, want)
	}
}
