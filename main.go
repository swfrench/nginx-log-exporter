package main

import (
	"flag"
	"log"
	"log/syslog"
	"net/http"
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

	useMetadataService = flag.Bool("use_metadata_service", true, "If true, use the GCE instance metadata service to fetch instance name and zone name, which will be added as metric labels.")

	manualInstanceName = flag.String("manual_instance_name", "", "Instance name to use when metadata service is disabled or OnGCE() returns false.")

	manualZoneName = flag.String("manual_zone_name", "", "Zone name to use when metadata service is disabled or OnGCE() returns false.")
)

func getLabelsFromMetadata() map[string]string {
	labels := make(map[string]string)

	if metadata.OnGCE() && *useMetadataService {
		instance, err := metadata.InstanceName()
		if err != nil {
			log.Fatalf("Could not retrieve instance name from metadata service: %v", err)
		}
		labels["instance_id"] = instance

		zone, err := metadata.Zone()
		if err != nil {
			log.Fatalf("Could not retrieve zone name from metadata service: %v", err)
		}
		labels["zone"] = zone
	} else {
		if *manualInstanceName == "" {
			log.Fatalf("Metadata service is disabled or not available, but default_instance_name is not set.")
		}
		labels["instance_id"] = *manualInstanceName

		if *manualZoneName == "" {
			log.Fatalf("Metadata service is disabled or not available, but default_zone_name is not set.")
		}
		labels["zone"] = *manualZoneName
	}
	return labels
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

	labels := getLabelsFromMetadata()

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
