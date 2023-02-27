# QSFS docker

## Important paths

The container mounts qsfs on `/mnt` and the 0-stor config is read from `/data/zstor.toml`.

Make sure to mount a valid 0-stor config in the container.

### 0-stor config

In the 0-stor config, set the following properties to these fixed paths:

```toml
root = "/mnt"
socket = "/var/run/zstor.sock"
zdb_data_dir_path = "/data/data"
```

## IPv6

When the 0-db's are deployed on the grid, they have IPv6 addresses so the [Docker daemon needs to be configured for this](https://docs.docker.com/config/daemon/ipv6/).

Note:
> IPv6 networking is only supported on Docker daemons running on Linux hosts.

## Building

```bash
docker build . -t qsfs
```

## Running

```bash
docker run -v <zstor.toml-path-file-on-host>:/data/zstor.toml -ti qsfs
```

Here is a sample of a [zstor.toml](./zstor-sample.toml)
