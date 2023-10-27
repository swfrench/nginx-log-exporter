package main

import (
	"flag"
	"fmt"
	"log"
	"log/syslog"
	"net/http"
	"strings"
	"time"

	"github.com/swfrench/nginx-log-exporter/internal/consumer"
	"github.com/swfrench/nginx-log-exporter/internal/file"
	"github.com/swfrench/nginx-log-exporter/internal/metrics"

	"cloud.google.com/go/compute/metadata"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	exportAddress = flag.String("export_address", "0.0.0.0:9091", "Address to which we export the /metrics handler.")

	accessLogPath = flag.String("access_log_path", "", "Path to access log file.")

	accessLogFormat = flag.String("access_log_format", "JSON", "Format of log lines in the access log. Supported: JSON (see README) and CLF.")

	logPollingPeriod = flag.Duration("log_polling_period", 30*time.Second, "Period between checks for new log lines.")

	rotationCheckPeriod = flag.Duration("rotation_check_period", time.Minute, "Idle period between log rotation checks.")

	useSyslog = flag.Bool("use_syslog", false, "If true, emit info logs to syslog.")

	useMetadataServiceLabels = flag.Bool("use_metadata_service_labels", false, "If true, use the GCE instance metadata service to fetch \"instance_id\" and \"zone\" labels, which will be applied to all metrics.")

	customLabels = flag.String("custom_labels", "", "A comma-separated, key=value list of additional labels to apply to all metrics.")

	monitoredPaths = flag.String("monitored_paths", "", "A comma-separated list of paths for which response metrics will be exported at path/method granularity. Paths are matched verbatim to the start of the first non-path expression (query string, fragment, etc.). Elements must be non-empty and contain no whitespace.")
)

func parseCustomLabels() (map[string]string, error) {
	labels := make(map[string]string)

	if len(*customLabels) > 0 {
		for _, elem := range strings.Split(*customLabels, ",") {
			if pair := strings.Split(elem, "="); len(pair) == 2 {
				labels[pair[0]] = pair[1]
			} else {
				return nil, fmt.Errorf("could not parse key=value pair: %v", elem)
			}
		}
	}

	return labels, nil
}

func parseMonitoredPaths() ([]string, error) {
	var paths []string

	if len(*monitoredPaths) > 0 {
		for _, elem := range strings.Split(*monitoredPaths, ",") {
			if len(elem) > 0 && len(strings.Fields(elem)) == 1 {
				paths = append(paths, elem)
			} else {
				return nil, fmt.Errorf("monitored paths must be non-empty and contain no whitespace")
			}
		}
	}

	return paths, nil
}

func getLabelsFromMetadataService() (map[string]string, error) {
	if !metadata.OnGCE() {
		return nil, fmt.Errorf("metadata service is unavailable when not on GCE")
	}

	instance, err := metadata.InstanceName()
	if err != nil {
		return nil, fmt.Errorf("could not retrieve instance name from metadata service: %v", err)
	}

	zone, err := metadata.Zone()
	if err != nil {
		return nil, fmt.Errorf("could not retrieve zone name from metadata service: %v", err)
	}

	return map[string]string{
		"instance_id": instance,
		"zone":        zone,
	}, nil
}

func main() {
	flag.Parse()

	if *useSyslog {
		w, err := syslog.New(syslog.LOG_INFO, "nginx_log_consumer")
		if err != nil {
			log.Fatalf("Could not create syslog writer: %v", err)
		}
		log.SetOutput(w)
	}

	t, err := file.NewTailer(*accessLogPath, *rotationCheckPeriod)
	if err != nil {
		log.Fatalf("Could not create tailer for %s: %v", *accessLogPath, err)
	}

	labels, err := parseCustomLabels()
	if err != nil {
		log.Fatalf("Could not parse custom labels: %v", err)
	}

	if *useMetadataServiceLabels {
		metadataLabels, err := getLabelsFromMetadataService()
		if err != nil {
			log.Fatalf("Could not fetch labels from metadata service: %v", err)
		}
		for key := range metadataLabels {
			labels[key] = metadataLabels[key]
		}
	}

	paths, err := parseMonitoredPaths()
	if err != nil {
		log.Fatalf("Could not parse monitored paths: %v", err)
	}

	log.Printf("Creating metrics manager for with base labels: %v", labels)

	m := metrics.NewManager(labels)

	log.Printf("Starting prometheus exporter at %s", *exportAddress)

	go func() {
		http.Handle("/metrics", promhttp.Handler())
		log.Fatal(http.ListenAndServe(*exportAddress, nil))
	}()

	c, err := consumer.NewConsumer(*logPollingPeriod, t, m, paths, *accessLogFormat)
	if err != nil {
		log.Fatalf("Could not create consumer: %v", err)
	}

	log.Printf("Starting consumer for %s", *accessLogPath)

	if err := c.Run(); err != nil {
		log.Fatalf("Failure consuming logs: %v", err)
	}
}
