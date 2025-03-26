## Introduction

Quantum Safe Storage (also known as Quantum Safe Filesystem or QSFS) is a distributed data store that is designed to provide resilience, security, and good performance. It is implemented on the frontend as a FUSE filesystem (Filesystem in Userspace) that can be mounted on any Linux machine. All files written to this filesystem are then dispersed among a configurable number of backends, such that failure of X nodes or Y groups of nodes can be tolerated without losing any data.

The system can support petabytes of total capacity, and the frontend serves as a cache of configurable size. Data blocks are encrypted and dispersed using forward looking error correcting codes (FLECC). Once stored in the backends, blocks can be freed from the frontend to make room for fresh data once the cache is full. When that data is needed again, it is reconstructed and decrypted on the fly.

Thanks to the methods used, not even a quantum computer capable of breaking encryption can decode data stored in the backends.

## Components

There are three main components comprising QSFS. These components are:

- [0-db-fs](https://github.com/threefoldtech/0-db-fs) (also known as zdbfs) is what creates the FUSE mount (the actual user facing filesystem). This component is not aware of the backend operations like encryption and FLECC. Its main job is to expose the FUSE filesystem and store its data in a local zdb instance.
- [0-db](https://github.com/threefoldtech/0-db) (also known as zdb) is used both for the local cache db and also for the backend data stores. Zdb is a fast and efficient append-only key value database.
- [0-stor](https://github.com/threefoldtech/0-stor_v2) (also known as zstor) is responsible for the encryption and FLECC operations on data blocks and database indexes, storing them among the configured backends.

Each component is also capable of independent operation. For example, zstor can be used to store individual files of any kind. Zdb is a general purpose key value store compatible with a subset of Redis operations, and zdbfs can be used without any offloading of data to backends.

More information about these projects can be found in the repositories linked above.
