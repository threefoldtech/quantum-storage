# QSFS docker

## Important paths

The container mounts qsfs on `/mnt` and the 0-stor config is read from `/data/zstor.toml`.

Make sure to mount a valid 0-stor config in the container.

### 0-stor config

In the 0-stor config, set the following properties to these fixed paths:

```toml
zdbfs_mountpoint = "/mnt"
socket = "/var/run/zstor.sock"
zdb_data_dir_path = "/data/data"
```

## IPv6

When the 0-db's are deployed on the grid, they have IPv6 addresses so the [Docker daemon needs to be configured for this](https://docs.docker.com/config/daemon/ipv6/).

You can also use host networking  by passing `--network host`to make sure the processes in the container can access the zdb's

Note:
> IPv6 networking is only supported on Docker daemons running on Linux hosts.

## Fuse

Make sure the container host has the fuse kernel module loaded ( `modprobe fuse`).
Inside the container the fuse device needs to be available ( pass `--device /dev/fuse`) and the process inside the container needs to be authorised to use it ( use ``--privileged` or `--cap-add SYS_ADMIN`).

## Running

```bash
docker run  --network host --device /dev/fuse --privileged -v <zstor.toml-path-file-on-host>:/data/zstor.toml  ghcr.io/threefoldtech/qsfs
```

Here is a sample of a [zstor.toml](./zstor-sample.toml)

## Docs

Please for more details about the separate components of the docker image refer to the [documentation here](docker.md)
