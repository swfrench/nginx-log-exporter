# Export metrics from nginx access logs to Prometheus

A small utility for exporting metrics inferred from nginx access logs to
Prometheus. Largely a re-spin of https://github.com/swfrench/nginx-log-consumer
for the (much simpler) pull-based API of Prometheus.

This is still pretty GCE-flavored, in that the exporter will attempt to read
the VM instance name and zone from the Metadata Service at start. If not on
GCE, these values can be set manually.

Currently only supports HTTP response status code counts, but it will be pretty
straightforward to add more.

## Requirements

### Dependencies

Run:

    go get -u cloud.google.com/go/compute/metadata
    go get -u github.com/prometheus/client_golang/prometheus

to pull the Metadata service and prometheus go client into your `GOPATH`.

### Log format

It is expected that nginx has been configured to write logs as json with ISO
8601 timestamps. For example:

    log_format json_combined escape=json '{ "time": "$time_iso8601", '
        '"remote_addr": "$remote_addr", '
        '"remote_user": "$remote_user", '
        '"request": "$request", '
        '"status": "$status", '
        '"body_bytes_sent": "$body_bytes_sent", '
        '"request_time": "$request_time", '
        '"http_referrer": "$http_referer", '
        '"http_user_agent": "$http_user_agent" }';
    access_log /var/log/nginx/access.log json_combined;

As noted above, only the `time` and `status` fields are examined for now.
