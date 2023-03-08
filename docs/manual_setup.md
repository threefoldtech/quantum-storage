# Manual setup of QSFS

This document explains how to manually set up QSFS on ubuntu. When using another linux distribution some steps can be slightly different.

## prerequisistes

### fuse

Install the fuse kernel module : `apt-get update && apt-get install fuse3`

### 0-db's

In order to sstore the data in remote locations, you need to have 0-db's.
4 0-db's are needed for the metadata and m+n for the data, n being the number of 0-db's that can be lost without data loss ( m 0-db's are required to restore the data ).

TODO: Link to how to install 0-db's on the grid.

If the 0-db's are deployed on the grid, make sure you can connect to the Theefold Planetary network.
This is already working if you are setting up qsfs on a VM on the grid, if not this needs to be configured (TODO: Link to docs).

## Directories

A qsfs mount point is required and a directory for qsfs to store the temporary data.
This guide assumes `/mnt/qsfs` for the mount point and `/data` for the qsfs temporary data. Create them if they do not exist yet.

## Install the individual components

`wget` the latest released binaries from the following components:

- 0-db-fs: <https://github.com/threefoldtech/0-db-fs/releases>: take the `amd64-linux-static` binary and save at `/bin/0-db-fs`
- 0-db: <https://github.com/threefoldtech/0-db/releases>: take the static binary and sace at `/bin/0-db`
- 0-stor: <https://github.com/threefoldtech/0-stor_v2/releases>: take `linux-musl` binary and save at `/bin/zstor`

Make sure all binaries are executable:`chmod a+x /bin/0-db-fs /bin/0-db /bin/zstor`

## 0-stor

Adapt the [example zstor configuration](./example_zstor_config.toml) to use the previously created 0-db's, set an [encryption key](./encryption.md) and save it at `/etc/zstor_default/toml`.

Now `zstor` can be started: `/usr/local/bin/zstor -c /etc/zstor_default.toml monitor`. If you don't want the process to block your terminal, you can start it in the background: `nohup /tmp/zstor -c /etc/zstor_default.toml monitor &`.

## Local 0-db

First we will get the [hook script](../lib/zdb-hook.sh).  Download it to `/bin/zdb-hook.sh` and make sure it is executable (`chmod +x /bin/zdb-hook.sh`).

The local 0-db which is used by 0-db-fs can now be started:

```sh
/bin/0-db \
    --index /data/index \
    --data /data/data \
    --datasize 67108864 \
    --mode seq \
    --hook /bin/zdb-hook.sh \
    --background
```

## 0-db-fs

Finally, we will start 0-db-fs. This guides opts to mount the fuse filesystem in `/mnt/qsfs`.

```sh
/bin/0-db-fs /mnt/qsfs -o autons -o background
```

You should now have the qsfs filesystem mounted at `/mnt/qsfs`. As you write data, it will save it in the local 0-db, and it's data containers will be periodically encoded and uploaded to the backend data storage 0-db's.

## It does not work

Check the [troubleshooting guide](./troubleshooting.md).
