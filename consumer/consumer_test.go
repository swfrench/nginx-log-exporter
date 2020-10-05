package consumer_test

import (
	"bytes"
	"fmt"
	"math"
	"sort"
	"testing"
	"text/template"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/swfrench/nginx-log-exporter/consumer"
	"github.com/swfrench/nginx-log-exporter/file/mock_tailer"
	"github.com/swfrench/nginx-log-exporter/metrics/mock_metrics"
)

var (
	jsonLogTemplate *template.Template
	clfLogTemplate  *template.Template
)

const (
	floatEqRelativeTolerance = 1e-9
	jsonLogTemplateFormat    = "{\"time\": \"{{.Time}}\", \"status\": \"{{.Status}}\", \"request_time\": {{.RequestTime}}, \"request\": \"{{.Method}} {{.Path}} HTTP/1.1\", \"bytes_sent\": {{.BytesSent}}, \"some_other\": \"stuff\"}\n"
	clfLogTemplateFormat     = "127.0.0.1 - - [{{.Time}}] \"{{.Method}} {{.Path}} HTTP/1.1\" {{.Status}} {{.BytesSent}} some other stuff\n"
)

func init() {
	jsonLogTemplate = template.Must(template.New("jsonLogLine").Parse(jsonLogTemplateFormat))
	clfLogTemplate = template.Must(template.New("clfLogLine").Parse(clfLogTemplateFormat))
}

// Custom Matchers

func floatEq(a, b float64) bool {
	if scale := math.Max(math.Abs(a), math.Abs(b)); scale > 0 {
		return math.Abs(a-b)/scale < floatEqRelativeTolerance
	}
	return true
}

type FloatMatcher struct {
	want float64
}

func (f FloatMatcher) Matches(got interface{}) bool {
	gotFloat, ok := got.(float64)
	if !ok {
		return false
	}
	return floatEq(f.want, gotFloat)
}

func (f FloatMatcher) String() string {
	return fmt.Sprintf("is a float64 value approximately equal to %v", f.want)
}

func FloatEq(want float64) FloatMatcher {
	return FloatMatcher{want: want}
}

func floatElementsEq(a, b []float64, ordered bool) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if ordered {
		for i := range a {
			if !floatEq(a[i], b[i]) {
				return false
			}
		}
	} else {
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
	}
	return true
}

type FloatElementsMatcher struct {
	want    []float64
	ordered bool
}

func (m FloatElementsMatcher) Matches(got interface{}) bool {
	gotSlice, ok := got.([]float64)
	if !ok {
		return false
	}
	return floatElementsEq(m.want, gotSlice, m.ordered)
}

func (m FloatElementsMatcher) String() string {
	var order string
	if m.ordered {
		order = "same order"
	} else {
		order = "any order"
	}
	return fmt.Sprintf("is a []float64 containing elements approximately equal to %v (%s)", m.want, order)
}

func (m FloatElementsMatcher) AnyOrder() FloatElementsMatcher {
	m.ordered = false
	return m
}

func FloatElementsEq(want []float64) FloatElementsMatcher {
	m := FloatElementsMatcher{ordered: true}
	m.want = append(m.want, want...)
	return m
}

// Helpers

type logLine struct {
	Time        string
	Status      string
	RequestTime string
	BytesSent   string
	Method      string
	Path        string
}

func buildLogLine(format string, line logLine, buffer *bytes.Buffer) error {
	switch format {
	case "JSON":
		return jsonLogTemplate.Execute(buffer, line)
	case "CLF":
		return clfLogTemplate.Execute(buffer, line)
	}
	return fmt.Errorf("Unsupported log line format: %s", format)
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

type mockMetricsSet struct {
	responseCounts         *mock_metrics.MockCounterT
	responseCountsDetailed *mock_metrics.MockCounterT
	responseTime           *mock_metrics.MockHistogramT
	responseSize           *mock_metrics.MockHistogramT
}

func mockInit(ctrl *gomock.Controller) (*mock_tailer.MockTailerT, *mock_metrics.MockManagerT, *mockMetricsSet) {
	t := mock_tailer.NewMockTailerT(ctrl)
	m := mock_metrics.NewMockManagerT(ctrl)

	m.EXPECT().AddCounter("http_response_count", "Counts of responses by status code", []string{
		"status_code",
	}).Return(nil)

	m.EXPECT().AddCounter("detailed_http_response_count", "Counts of responses by status code, path, and method", []string{
		"status_code",
		"path",
		"method",
	}).Return(nil)

	m.EXPECT().AddHistogram("http_response_time", "Response time (seconds) by status code", []string{
		"status_code",
	}, gomock.Nil()).Return(nil)

	m.EXPECT().AddHistogram("http_response_bytes_sent", "Response size (bytes) by status code", []string{
		"status_code",
	}, FloatElementsEq([]float64{8, 16, 64, 128, 256, 512, 1024, 2048, 4096})).Return(nil)

	s := &mockMetricsSet{
		responseCounts:         mock_metrics.NewMockCounterT(ctrl),
		responseCountsDetailed: mock_metrics.NewMockCounterT(ctrl),
		responseTime:           mock_metrics.NewMockHistogramT(ctrl),
		responseSize:           mock_metrics.NewMockHistogramT(ctrl),
	}

	m.EXPECT().GetCounter("http_response_count").AnyTimes().Return(s.responseCounts, nil)
	m.EXPECT().GetCounter("detailed_http_response_count").AnyTimes().Return(s.responseCountsDetailed, nil)
	m.EXPECT().GetHistogram("http_response_time").AnyTimes().Return(s.responseTime, nil)
	m.EXPECT().GetHistogram("http_response_bytes_sent").AnyTimes().Return(s.responseSize, nil)

	return t, m, s
}

// Tests

func testWithoutDetailedCountsBase(format, timeExample string, t *testing.T) {
	const testPeriod = 10 * time.Millisecond

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tailer, manager, metricsSet := mockInit(ctrl)

	minCreationTime := time.Now()
	c, err := consumer.NewConsumer(testPeriod, tailer, manager, []string{}, format)
	if err != nil {
		t.Fatalf("Could not build new consumer: %v", err)
	}
	maxCreationTime := time.Now()

	timeEarly := minCreationTime.Add(-1 * time.Minute).Format(timeExample)
	timeLate := maxCreationTime.Add(time.Minute).Format(timeExample)

	var buffer bytes.Buffer
	for _, line := range []logLine{
		{
			Time:        timeEarly,
			Status:      "200",
			RequestTime: "0.010",
			BytesSent:   "100",
			Method:      "GET",
			Path:        "/",
		},
		{
			Time:        timeLate,
			Status:      "200",
			RequestTime: "0.020",
			BytesSent:   "200",
			Method:      "GET",
			Path:        "/foo",
		},
		{
			Time:        timeLate,
			Status:      "200",
			RequestTime: "0.030",
			BytesSent:   "300",
			Method:      "POST",
			Path:        "/foo",
		},
		{
			Time:        timeLate,
			Status:      "500",
			RequestTime: "0.040",
			BytesSent:   "400",
			Method:      "GET",
			Path:        "/foo?bar=1",
		},
	} {
		buildLogLine(format, line, &buffer)
	}

	gomock.InOrder(
		tailer.EXPECT().Next().Times(1).Return(buffer.Bytes(), nil),
		tailer.EXPECT().Next().AnyTimes().Return([]byte{}, nil),
	)

	metricsSet.responseCounts.EXPECT().Add(map[string]string{"status_code": "200"}, FloatEq(2)).Return(nil)
	metricsSet.responseCounts.EXPECT().Add(map[string]string{"status_code": "500"}, FloatEq(1)).Return(nil)

	metricsSet.responseCountsDetailed.EXPECT().Add(gomock.Any(), gomock.Any()).Times(0)

	// Plain CLF does not export response time.
	if format == "CLF" {
		metricsSet.responseTime.EXPECT().Observe(gomock.Any(), gomock.Any()).Times(0)
	} else {
		metricsSet.responseTime.EXPECT().Observe(map[string]string{"status_code": "200"}, FloatElementsEq([]float64{0.02, 0.03})).Return(nil)
		metricsSet.responseTime.EXPECT().Observe(map[string]string{"status_code": "500"}, FloatElementsEq([]float64{0.04})).Return(nil)
	}

	metricsSet.responseSize.EXPECT().Observe(map[string]string{"status_code": "200"}, FloatElementsEq([]float64{200, 300})).Return(nil)
	metricsSet.responseSize.EXPECT().Observe(map[string]string{"status_code": "500"}, FloatElementsEq([]float64{400})).Return(nil)

	testRunConsumer(t, c)
}

func TestWithoutDetailedCountsJson(t *testing.T) {
	testWithoutDetailedCountsBase("JSON", consumer.ISO8601, t)
}

func TestWithoutDetailedCountsClf(t *testing.T) {
	testWithoutDetailedCountsBase("CLF", consumer.CLF, t)
}

func testWithDetailedCountsBase(format, timeExample string, t *testing.T) {
	const testPeriod = 10 * time.Millisecond

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tailer, manager, metricsSet := mockInit(ctrl)

	minCreationTime := time.Now()
	c, err := consumer.NewConsumer(testPeriod, tailer, manager, []string{
		"/foo",
		"/bar",
	}, format)
	if err != nil {
		t.Fatalf("Could not build new consumer: %v", err)
	}
	maxCreationTime := time.Now()

	timeEarly := minCreationTime.Add(-1 * time.Minute).Format(timeExample)
	timeLate := maxCreationTime.Add(time.Minute).Format(timeExample)

	var buffer bytes.Buffer
	for _, line := range []logLine{
		{
			Time:        timeEarly,
			Status:      "200",
			RequestTime: "0.010",
			BytesSent:   "100",
			Method:      "GET",
			Path:        "/",
		},
		{
			Time:        timeLate,
			Status:      "200",
			RequestTime: "0.020",
			BytesSent:   "200",
			Method:      "GET",
			Path:        "/foo",
		},
		{
			Time:        timeLate,
			Status:      "200",
			RequestTime: "0.030",
			BytesSent:   "300",
			Method:      "POST",
			Path:        "/foo",
		},
		{
			Time:        timeLate,
			Status:      "200",
			RequestTime: "0.040",
			BytesSent:   "400",
			Method:      "GET",
			Path:        "/foo",
		},
		{
			Time:        timeLate,
			Status:      "500",
			RequestTime: "0.050",
			BytesSent:   "500",
			Method:      "GET",
			Path:        "/bar?baz=1",
		},
		{
			Time:        timeLate,
			Status:      "500",
			RequestTime: "0.060",
			BytesSent:   "600",
			Method:      "GET",
			Path:        "/baz",
		},
	} {
		buildLogLine(format, line, &buffer)
	}

	gomock.InOrder(
		tailer.EXPECT().Next().Times(1).Return(buffer.Bytes(), nil),
		tailer.EXPECT().Next().AnyTimes().Return([]byte{}, nil),
	)

	metricsSet.responseCounts.EXPECT().Add(map[string]string{"status_code": "200"}, FloatEq(3)).Return(nil)
	metricsSet.responseCounts.EXPECT().Add(map[string]string{"status_code": "500"}, FloatEq(2)).Return(nil)

	metricsSet.responseCountsDetailed.EXPECT().Add(map[string]string{
		"status_code": "200",
		"path":        "/foo",
		"method":      "GET",
	}, FloatEq(2)).Return(nil)
	metricsSet.responseCountsDetailed.EXPECT().Add(map[string]string{
		"status_code": "200",
		"path":        "/foo",
		"method":      "POST",
	}, FloatEq(1)).Return(nil)
	metricsSet.responseCountsDetailed.EXPECT().Add(map[string]string{
		"status_code": "500",
		"path":        "/bar",
		"method":      "GET",
	}, FloatEq(1)).Return(nil)

	// Plain CLF does not export response time.
	if format == "CLF" {
		metricsSet.responseTime.EXPECT().Observe(gomock.Any(), gomock.Any()).Times(0)
	} else {
		metricsSet.responseTime.EXPECT().Observe(map[string]string{"status_code": "200"}, FloatElementsEq([]float64{0.02, 0.03, 0.04})).Return(nil)
		metricsSet.responseTime.EXPECT().Observe(map[string]string{"status_code": "500"}, FloatElementsEq([]float64{0.05, 0.06})).Return(nil)
	}

	metricsSet.responseSize.EXPECT().Observe(map[string]string{"status_code": "200"}, FloatElementsEq([]float64{200, 300, 400})).Return(nil)
	metricsSet.responseSize.EXPECT().Observe(map[string]string{"status_code": "500"}, FloatElementsEq([]float64{500, 600})).Return(nil)

	testRunConsumer(t, c)
}

func TestWithDetailedCountsJson(t *testing.T) {
	testWithDetailedCountsBase("JSON", consumer.ISO8601, t)
}

func TestWithDetailedCountsClf(t *testing.T) {
	testWithDetailedCountsBase("CLF", consumer.CLF, t)
}
