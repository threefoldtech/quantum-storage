# Quantum Storage Filesystem

Quantum Storage is a FUSE filesystem that uses mechanisms of forward looking error correcting codes to make sure data (files and metadata) are stored in multiple remote places in a way that we can afford losing `x` number of locations without losing the data. There is other factors that are involved into this operation like encryption. Please check [0-stor](https://github.com/threefoldtech/0-stor_v2) documentations for details.

The aim is to support unlimited local storage with remote backends for offload and backup which cannot be broken, even by a quantum computer.

## Overview

To have a working qsfs filesystem there are multiple components that need to work together to make it work. These components are

- [0-db-fs](https://github.com/threefoldtech/0-db-fs) this is what creates the `FUSE` mount (the actual user facing filesystem) this component does not know about qsfs or forward error correction. It's main job is to expose the fuse filesystem and store it's data in a local zdb instacne
- [0-db](https://github.com/threefoldtech/0-db) is a local `cache` db. this is what is used by the `0-db-fs` to store the actual data of the filesystem. This means that any read/write operations triggered by the `0-db-fs` directly access this (single) instance of `0-db` for the data blocks
- [0-stor](https://github.com/threefoldtech/0-stor_v2) zero stor is listening to `0-db` events (with a hooks system) to upload and/or download zdb data files segments to remote locations. that's where the encryption and forward error correction happens.

Since zdb is an `append-only` database, the local db will just keep growing linearly with each write (and delete) operation. ZDB will then create db segment files that are guaranteed to **not** change in the future. What happens once a segment file is closed (it hit it's max file size) a hook is triggered which in return will trigger `0-stor` to chunk and upload this file to the remote locations (zdbs).

The segment file will then be deleted (at some point) in the future when the number of segment files reaches a certain number, older files will get deleted.

If the filesystem then is trying to access a piece of old data, it will make a read call to the local `zdb`. If the zdb is trying to access an old segment of the db that is no longer on disk, another hook is triggered to `0-stor` to download that segment. 0-stor then will re-download the required segment from the remote locations and re-build it.

Once the zdb segment file is restored, the read operation continues.

## How to deploy

It's possible to run an entirely local instance for testing. See details below on how to do this with Docker.

Creating deployments with remote backends is the main purpose of this system. That's easiest to do on the ThreeFold Grid, where HDD based zdb capacity can be rented as a primitive. There are two main deployment methods for the Grid documented on this repo:

* [Tfcmd](https://github.com/threefoldtech/quantum-storage/blob/master/docs/04_deploy_tfcmd.md) (more manual)
* [Pulumi](https://github.com/threefoldtech/quantum-storage/blob/master/docs/04_deploy_tfcmd.md) (more automated)

The docs also contain more information about how the system works and how to plan a deployment, starting with an [overview](https://github.com/threefoldtech/quantum-storage/blob/master/docs/01_overview.md).

## Local deployment using Docker

There's also a Dockerfile under `docker` that spins up a mock full QSFS deployment locally in a single container. It's meant for testing and could also be useful for studying the system. This is the most compact example to see the end to end process of fitting all the components together.
