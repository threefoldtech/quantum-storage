# Quantum Storage Filesystem

The Quantum Storage Filesystem is a FUSE filesystem which aim to be able to support unlimited local storage
with remote backend for offload and backup which cannot be broke even by a quantum computer.

## Summary

1. [Components](#components)
2. [Bootstrap](#bootstrap)
3. [Extra feature](#extra-feature)

## Components

The full chain about quantum storage is made of 3 parts:

- [0-db](https://github.com/threefoldtech/0-db): storage engine
- [0-db-fs](https://github.com/threefoldtech/0-db-fs): FUSE layer which use the storage engine in an optimized way
- [0-stor-v2](https://github.com/threefoldtech/0-stor_v2): engine to save/backup data to safe location

### 0-db-fs

This is a simple filesystem driver which use 0-db as primary storage engine.

Directories and metadata are stored in a dedicated namespace, file payloads are saved in another
dedicated namespace.

The filesystem use an internal cache system made, not for performance, but to optimize how data
are stored in the 0-db to avoid overhead as much as possible.

### 0-db

This is an always append database, which store object in an immuable format, which allows to
have history out-of-box, good performance on disk, low overhead, easy data structure, easy backup
(linear copy and immuable files).

We use two type of 0-db: one for the storage backend next to 0-db-fs, which is where data needs
to be, to be available via the fuse filesystem. This 0-db can offload it's data to an external process
(user-defined) and can also request missing data to be retreived, if some data are missing.

This enable the database to spread and not using always local storage space if data are not read.

One external process to handle theses cases is 0-stor-v2 we use.

### 0-stor-v2

This tool can be used as external process for 0-db, or can be used for any purpose. It just takes one file
as input, it encrypt this file in AES based on a key user-defined, encode file and spread them
to multiple 0-db. Based on the policy (configurable), some amount of 0-db can be unreachable, data can still
retreived and missing database can even be rebuilt to keep full consistance available.

Metadata needed to get data back from 0-db in order, are stored in others 0-db.

## Bootstrap

You can use the bootstrap (`bootstrap/bootstrap.v`) to download and starts required components and start
everything required. Default configuration use everything localy. You can pass a specific zstor configuration file
to use a real backend out-of-box.

Everything will be installed in `~/.threefold` and nowhere else.
This bootstrap will spawn two `zdb`, one `zstor daemon` and the `zdbfs` fuse system.

## Extra Feature

You can use a special option with docker to mount-share the container mountpoint:

```bash
mkdir /mnt/zdbfs
docker run [...] --mount type=bind,source=/mnt/zdbfs,target=/mnt,bind-propagation=rshared tf/quantum
```

Using this feature, you will get the `/mnt/zdbfs` on your host, being the same mount as `/mnt` inside
the container, which will give you `0-db-fs` feature available on your host directly.

So anything going to `/mnt/zdbfs` on your host, is sent to zdbfs process.
