# Export metrics from nginx access logs to Prometheus

![build-and-test status](https://github.com/swfrench/nginx-log-exporter/actions/workflows/build-and-test.yml/badge.svg)

A small utility for exporting metrics inferred from nginx access logs to
[Prometheus](https://prometheus.io).

## Metrics

The following metrics are currently supported:

*   `nginx_http_response_total` - Total response count, by HTTP response code
*   `nginx_http_response_detailed_total` - "Detailed" total response count, by
     HTTP response code, method, and path (only enabled for a configurable set
     of exact-match paths; see `-monitored_paths`)
*   `nginx_http_response_duration_seconds` - Response duration (i.e., request
    processing latency) distribution, by HTTP response code
*   `nginx_http_response_size_bytes` - Response size (i.e., bytes sent, headers
    inclusive) distribution, by HTTP response code

## Building

`go get github.com/swfrench/nginx-log-exporter` will fetch all required
dependencies and build the `nginx-log-exporter` binary (which will be placed in
your `$GOPATH/bin`).

If working from a local copy of this source tree, `go build` from the base
directory should suffice (producing a binary reflecting any local changes you
have made).

## Log format

Two access log formats are supported:

*   JSON: A custom format described in more detail below (default).
*   [Common Log Format](https://en.wikipedia.org/wiki/Common_Log_Format): CLF
    is the basic default format for nginx (well, really an extension thereof).
    Note that response time metrics are not supported under CLF.

Which format is expected by the exporter is controlled by the
`-access_log_format` flag (supported values being "CLF" and "JSON" with the
latter being the default).

If using the JSON log line format, nginx should be configured to write access
logs with _at least_ the following fields present (additional fields are fine,
and will be ignored):

    # Example minimal log format
    log_format json_combined escape=json '{ '
        '"time": "$time_iso8601", '
        '"request": "$request", '
        '"status": "$status", '
        '"bytes_sent": $bytes_sent, '
        '"request_time": $request_time }';
    access_log /var/log/nginx/access.log json_combined;

**Note:** The `escape` parameter for `log_format` is only supported by nginx
1.11.8 and later.

## Running on GCE

If running in a GCE VM instance, you can set the `-use_metadata_service_labels`
flag to pull the instance name and zone from the Metadata service, which will
in turn be added to your metrics (along with any additional key=value label
pairs provided via the `-custom_labels` flag).
