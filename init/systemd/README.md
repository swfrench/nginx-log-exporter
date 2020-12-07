# Systemd setup

1. Create an unprivileged system user `nginx_log_exporter` with read
   permissions for nginx access logs. For example, on a Debian-based
   distribution: `useradd -r nginx_log_exporter -s /usr/sbin/nologin` and
   `usermod -a -G adm nginx_log_exporter`.
2. Copy the binary to `/usr/sbin/nginx-log-exporter`.
3. Copy `nginx_log_exporter.service` to an appropriate location such that
   systemd can find it (e.g. `/etc/systemd/` or `/lib/systemd/`) and copy
   `nginx_log_exporter.config` to `/etc/default/nginx_log_exporter`.
4. Run `sudo systemctl enable nginx_log_exporter.service` and `sudo systemctl
   start nginx_log_exporter`.

You may want to examine the unit file and consider adjusting values to your use
case.
