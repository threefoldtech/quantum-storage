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

## Install the individual components

`wget` the latest released binaries from the following components:

- 0-db-fs: <https://github.com/threefoldtech/0-db-fs/releases>: take the `amd64-linux-static` binary and save at `/usr/local/bin/0-db-fs`
- 0-db: <https://github.com/threefoldtech/0-db/releases>: take the static binary and sace at `/usr/local/bin/0-db`
- 0-stor: <https://github.com/threefoldtech/0-stor_v2/releases>: take `linux-musl` binary and save at `/usr/local/bin/zstor`

Make sure all binaries are executable:`chmod a+x /usr/local/bin/0-db-fs /usr/local/bin/0-db /usr/local/bin/zstor`

## It does not work

Check the [troubleshooting guide](./troubleshooting.md).
