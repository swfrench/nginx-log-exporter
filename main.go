package main

import (
	"flag"
	"fmt"
	"log"
	"log/syslog"
	"net/http"
	"strings"
	"time"

	"github.com/swfrench/nginx-log-exporter/consumer"
	"github.com/swfrench/nginx-log-exporter/exporter"
	"github.com/swfrench/nginx-log-exporter/tailer"

	"cloud.google.com/go/compute/metadata"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	exportAddress = flag.String("export_address", "0.0.0.0:9091", "Address to which we export the /metrics handler.")

	accessLogPath = flag.String("access_log_path", "", "Path to access log file.")

	logPollingPeriod = flag.Duration("log_polling_period", 30*time.Second, "Period between checks for new log lines.")

	rotationCheckPeriod = flag.Duration("rotation_check_period", time.Minute, "Idle period between log rotation checks.")

	useSyslog = flag.Bool("use_syslog", false, "If true, emit info logs to syslog.")

	useMetadataServiceLabels = flag.Bool("use_metadata_service_labels", false, "If true, use the GCE instance metadata service to fetch \"instance_id\" and \"zone\" labels, which will be applied to all metrics.")

	customLabels = flag.String("custom_labels", "", "A comma-separated, key=value list of additional labels to apply to all metrics.")
)

func parseCustomLabels() (map[string]string, error) {
	labels := make(map[string]string)

	if len(*customLabels) > 0 {
		for _, elem := range strings.Split(*customLabels, ",") {
			if pair := strings.Split(elem, "="); len(pair) == 2 {
				labels[pair[0]] = pair[1]
			} else {
				return nil, fmt.Errorf("Could not parse key=value pair: %v", elem)
			}
		}
	}

	return labels, nil
}

func getLabelsFromMetadataService() (map[string]string, error) {
	if !metadata.OnGCE() {
		return nil, fmt.Errorf("Metadata service is unavailable when not on GCE")
	}

	instance, err := metadata.InstanceName()
	if err != nil {
		return nil, fmt.Errorf("Could not retrieve instance name from metadata service: %v", err)
	}

	zone, err := metadata.Zone()
	if err != nil {
		return nil, fmt.Errorf("Could not retrieve zone name from metadata service: %v", err)
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

	t, err := tailer.NewTailer(*accessLogPath, *rotationCheckPeriod)
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

	log.Printf("Creating exporter for resource: %v", labels)

	e := exporter.NewExporter(labels)

	log.Printf("Starting prometheus exporter at %s", *exportAddress)

	go func() {
		http.Handle("/metrics", promhttp.Handler())
		log.Fatal(http.ListenAndServe(*exportAddress, nil))
	}()

	c := consumer.NewConsumer(*logPollingPeriod, t, e)

	log.Printf("Starting consumer for %s", *accessLogPath)

	if err := c.Run(); err != nil {
		log.Fatalf("Failure consuming logs: %v", err)
	}
}
