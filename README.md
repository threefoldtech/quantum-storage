# Quantum Storage

[todo: quantum storage short explaination]

# Summary

1. [Components](#components)
2. [Repository content](#repository-content)
3. [Execution](#execution)
4. [Init script](#init-script)
5. [Extra feature](#extra-feature)

# Components

The full chain about quantum storage is made of 3 parts:
- [0-db](https://github.com/threefoldtech/0-db): storage engine
- [0-db-fs](https://github.com/threefoldtech/0-db-fs): FUSE layer which use the storage engine in an optimized way
- [0-stor-v2](https://github.com/threefoldtech/0-stor_v2): erasure encode storage data and send chunks to safe location

## 0-db-fs

This is a simple filesystem driver which use 0-db as primary storage engine.

Directories and metadata are stored in a dedicated namespace, file payloads are saved in another
dedicated namespace.

The filesystem use an internal cache system made, not for performance, but to optimize how data
are stored in the 0-db to avoid overhead as much as possible.

## 0-db

This is an always append database, which store object in an immuable format, which allows to
have history out-of-box, good performance on disk, low overhead, easy data structure, easy backup
(linear copy and immuable files).

We use two type of 0-db: one for the storage backend next to 0-db-fs, which is where data needs
to be, to be available via the fuse filesystem. This 0-db can offload it's data to an external process
(user-defined) and can also request missing data to be retreived, if some data are missing.

This enable the database to spread and not using always local storage space if data are not read.

One external process to handle theses cases is 0-stor-v2 we use.

## 0-stor-v2

This tool can be used as external process for 0-db, or can be used for any purpose. It just takes one file
as input, it encrypt this file in AES based on a key user-defined, then erasure encode file and spread them
to multiple 0-db. Based on the policy (configurable), some amount of 0-db can be unreachable, data can still
retreived and missing database can even be rebuilt to keep full consistance available.

Metadata needed to get data back from 0-db in order, are stored in etcd, whcih can run in cluster.

# Repository Content

- `default-sample.toml`: default 0-stor-v2 configuration filled with some 0-db available to provide
quick and easy test feature
- `Dockerfile`: build script to get a quick and small Docker with minio on top on 0-db-fs
- `init.sh`: init script executed on container start, which basically runs everything needed on runtime
- `nscreate.v`: a small V program to create needed namespace inside 0-db needed by 0-db-fs
- `zdb-hook.sh`: hook script attached to 0-db, which is the 'user-defined' way to send data to 0-stor-v2 and
to retreive data aswell

# Execution

In order to get the container running with full power, you need to get ipv6 working inside your Docker.
You can enable ipv6 by editing: `/etc/docker/daemon.json`, see more information on Docker official documentation.

If you don't have ipv6 working, 0-fs-db will still works but you won't have the erasure coding working.

## Build

Build the docker image using:
```bash
docker build -t tf/quantum .
```

This will download, compile and prepare needed stuff to get a working container. Build is made inside an Ubuntu
container, but real runtime and execution container is Alpine, quite small. Only minimal stuff are pushed
to the image.

## Execution

You need to run the container with `FUSE` access enabled and mount privilege:
```bash
docker run --rm -it --device /dev/fuse --cap-add CAP_SYS_ADMIN tf/quantum
```

As soon as the container is up-and-running, you can reach it via it's ip, port 9000 to get
minio web interface. You maybe need to add `-p 9000:9000` if you want some port-forward.

# Init Script

If you want to deploy stuff yourself, here are some explaination on commands executed inside the container.

## 0-db
```bash
zdb \
  --datasize $((32 * 1024 * 1024)) \     # limit datafiles to 32MB
  --mode seq \                           # default running mode required by 0-db-fs
  --listen 127.0.0.1 \                   # specify listening interface, could be 0.0.0.0 aswell
  --hook /lib/zdb/zstor-hook.sh \        # user-defined script called when datafile are full or missing
  --data /zdb/data \                     # location where data are saved
  --index /zdb/index                     # location where index are saved
```

Data will be sent to the grid and erasure encoded only (for now) when a datafile is full. By setting data size
to 32 MB, basically, each time 32 MB are send to the fuse, theses data are sent to the grid and are safe forever.

Some fix later will push data even before reaching the limit to ensure time-based persistance aswell.

## 0-db-fs
```bash
nscreate       # create required default namespaces in 0-db, expected by 0-db-fs
zdbfs /mnt     # mount the filesystem into /mnt
```

## Cluster and Minio

There is a default etcd running and minio. The etcd can run alone or in cluster, it's up to you
to configure that, if it runs alone, obviously if it dies, you will loose your grid metadata.

# Extra Feature

You can use a special option with docker to mount-share the container mountpoint:
```bash
mkdir /mnt/zdbfs
docker run [...] --mount type=bind,source=/mnt/zdbfs,target=/mnt,bind-propagation=rshared tf/quantum
```

Using this feature, you will get the `/mnt/zdbfs` on your host, being the same mount as `/mnt` inside
the container, which will give you `0-db-fs` feature available on your host directly.

So anything going to `/mnt/zdbfs` on your host, is sent to zdbfs process.
