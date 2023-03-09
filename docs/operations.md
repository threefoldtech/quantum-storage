# QSFS Operation

## zstor status

## commandline

the `zstor -c <zstor_config.toml> status` command gives an overview of the backend 0-db's, if they are reachable or not and the storage space they consume.

## Monitoring, alerting and statistics

0-stor collects metrics about the system. It can be configured with a 0-db-fs mountpoint, which will trigger 0-stor to collect 0-db-fs statistics, next to some 0-db statistics which are always collected. If the prometheus_port config option is set, 0-stor will serve metrics on this port for scraping by prometheus.

They are available over http on the configured port at the `/metrics` path.
Test it with `curl localhost:9100/metrics` for example.

## A 0-db backend is broken

If a 0-db backend is broken (the host is down for example) it needs to be replaced.

Shut down the entire qsfs system, replace the malfunctioning 0-db with a new one in the zstor config and start everything again.

The 0-stor repair subsystem will take care of rebulding the data, regenerating the shards, and storing the new shards on the new 0-db.
