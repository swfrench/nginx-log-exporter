# Export metrics from nginx access logs to Prometheus

[![main build status](https://travis-ci.com/swfrench/nginx-log-exporter.svg?branch=main)](https://travis-ci.com/swfrench/nginx-log-exporter)

A small utility for exporting metrics inferred from nginx access logs to
[Prometheus](https://prometheus.io).

## Metrics

The following metrics are currently supported:

*   Response counts, by HTTP response code
*   "Detailed" response counts, by HTTP response code, method, and path (only
    enabled for a configurable set of exact-match paths; see
    `-monitored_paths`)
*   Response time (i.e. request processing latency) distribution, by HTTP
    response code

## Building

`go get github.com/swfrench/nginx-log-exporter` will fetch all required
dependencies and build the `nginx-log-exporter` binary (which will be placed in
your `$GOPATH/bin`).

If working from a local copy of this source tree, `go build` from the base
directory should suffice (producing a binary reflecting any local changes you
have made).

## Log format

It is expected that nginx has been configured to write logs as json with ISO
8601 timestamps. A minimal example (you'll probably want more fields):

    log_format json_combined escape=json '{ '
        '"time": "$time_iso8601", '
        '"request": "$request", '
        '"status": "$status", '
        '"bytes_sent": $bytes_sent, '
        '"request_time": $request_time }';
    access_log /var/log/nginx/access.log json_combined;

For now, only the `time`, `status`, `request_time`, `bytes_sent`, and `request`
(for detailed path / method metrics) fields are examined (any others will be
ignored).

**Note:** The `escape` parameter for `log_format` is only supported by nginx
1.11.8 and later.

## Running on GCE

If running on GCE, you can set `-use_metadata_service_labels` to pull the
instance name and zone from the Metadata service, which will in turn be added
to your metrics.
