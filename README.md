# Export metrics from nginx access logs to Prometheus

![master build status](https://travis-ci.org/swfrench/nginx-log-exporter.svg?branch=master)

A small utility for exporting metrics inferred from nginx access logs to
[Prometheus](https://prometheus.io).

Similar in concept to
[nginx-log-consumer](https://github.com/swfrench/nginx-log-consumer), but
Prometheus flavored, rather than tied to the Stackdriver Monitoring API.

## Metrics

The following metrics are currently supported:

*   Response counts, by HTTP response code
*   "Detailed" response counts, by HTTP response code, method, and path (only
    enabled for exact-match whitelisted paths; see `-monitored_paths`)
*   Response time (i.e. request processing latency) distribution, by HTTP
    response code

## Requirements

### Build

Run:

    go get -u github.com/swfrench/nginx-log-exporter

to build the exporter, which should now be in `$GOPATH/bin` (this will also
pull in transitive dependencies, such as the Metadata service and Prometheus go
client).

### Log format

It is expected that nginx has been configured to write logs as json with ISO
8601 timestamps. For example:

    log_format json_combined escape=json '{ "time": "$time_iso8601", '
        '"remote_addr": "$remote_addr", '
        '"remote_user": "$remote_user", '
        '"request": "$request", '
        '"status": "$status", '
        '"body_bytes_sent": "$body_bytes_sent", '
        '"request_time": $request_time, '
        '"http_referrer": "$http_referer", '
        '"http_user_agent": "$http_user_agent" }';
    access_log /var/log/nginx/access.log json_combined;

For now, only the `time`, `status`, `request_time`, and `request` (for detailed
path / method metrics) are examined.

**Note:** The `escape` parameter for `log_format` is only supported by nginx
1.11.8 and later.

## Running on GCE

If running on GCE, you can set `-use_metadata_service_labels` to pull the
instance name and zone from the Metadata service, which will in turn be added
to your metrics.
