> TODO: Update with quantumd specific info.

## Introduction

While some operational details are covered in the deployment sections above, this section provides an overview of operational concerns for the Quantum Safe Storage system.

## Data rotation

Data is uploaded from the frontend machine to the backends in two cases:

1. A zdbfs data block is filled
2. The rotation timeout is reached (specified as zdbfs cli flag)

This means that the rotation timeout, plus time spent processing and uploading data, represents an upper limit on how long data sent to the frontend might be lost if the frontend goes down.

While a shorter rotation timeout means less potential for data loss, a longer timeout can mean that more data blocks get completely filled which is better for zdb performance. A timeout of 15 minutes is probably a good compromise for most use cases.

## Monitoring

Zstor exposes various metrics on a Prometheus endpoint, including metrics about the backends, zstor operationns, and also about the zdbfs process.

Metrics are served at `localhost:9100/metrics`.

These are the available metrics of each type:

### Backend

  - Status of the connection to the 0-db
  - Number of entries stored
  - Size of data in bytes
  - Data limit in bytes
  - Size of index in bytes
  - Number of I/O errors in index
  - Number of faults in index
  - Number of I/O errors in data
  - Number of faults in data
  - Free space in bytes on index disk
  - Free space in bytes on data disk

### Zstor

  - Store commands that finished
  - Retrieve commands that finished
  - Rebuild commands that finished
  - Check commands that finished

### Zdbfs

  - Total amount of fuse requests
  - Total amount of cache hits in the filesystem
  - Total amount of cache misses in the filesystem
  - Total amount of times the cache was completely filled
  - Total amount of linear flushes
  - Total amount of random flushes
  - Total amount of cache branches
  - Amount of cache branches allocated
  - Amount of memory cache entries
  - Amount of blocks in the memory cache
  - Amount of bytes used by cache blocks
  - Total amount of syscalls done on the filesystem
  - Total amount of bytes read from the filessytem
  - Total amount of bytes written to the filessytem
  - Total amount of errors returned by fuse calls

## Visualize monitoring data with Grafana

If you connect a Grafana instance to the Prometheus instance hosting metrics from zstor, you can import [this dashboard](https://scottyeager.grafana.net/public-dashboards/b522da8a37864e86bcc384ebdc5ae74e) to visualize the data. Note that the current dashboard is a first version and is not necessarily complete or optimal.

## Maintenance

If a backend goes offline, it can be replaced with a new one. Here are the steps to do this manually:

1. Deploy new zdb with same or greater storage capacity
2. Update zstor config to use new zdb
3. Hot reload zstor config by issuing a `SIGUSR1` signal with `kill -SIGUSR1` (restarting zstor also works)
4. Zstor repair subsystem will automatically regenerate the data shards and store them in the new backend zdb.

## Zstor

There are a few commands available for querying info from zstor on the cli.

### Status

This shows an overview of the configured backends and their status:

```
zstor -c /path/to/config status
```

### Check

The `check` command simply checks that a given path has an entry in the metadata store. It does not actually check the integrity of the data.

```
zstor -c /path/to/config check /path/to/file
```

The file's checksum is printed, along with the path, if the metadata exists.

Here's a command to generate the same checksum independently:

```
b2sum -l 128 /path/to/file
```
