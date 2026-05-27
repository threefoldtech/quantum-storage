# QSS Quantum Storage

QSS Quantum Storage is a FUSE filesystem that uses forward error correction and encryption to distribute data across multiple remote storage locations. It is designed so that a configurable number of locations can be lost without compromising data availability or integrity. The system is built to resist cryptographic attacks from quantum computers by using quantum-safe principles. For details on the underlying storage engine, see the [0-stor](https://github.com/threefoldtech/0-stor_v2) documentation.

## What this is

QSS Quantum Storage provides a local filesystem interface backed by distributed remote storage. Data and metadata are split, encrypted, and spread across multiple backends using forward error correcting codes. If some backends become unreachable, the remaining pieces are sufficient to reconstruct the original data. The local component caches recently used data for fast access, while older segments are offloaded to remote backends.

## What this repository contains

- **QSS Quantum Storage filesystem components** integrating FUSE, local caching, and remote distribution
- **Documentation** on system architecture, configuration, and usage in the [docs](./docs) directory
- **Integration with 0-db-fs, 0-db, and 0-stor** for the complete storage pipeline

## Role in the stack

QSS Quantum Storage operates as a distributed storage layer within the broader infrastructure stack. It relies on the following components:

- **[0-db-fs](https://github.com/threefoldtech/0-db-fs)** — Exposes the FUSE mount point to users. It stores filesystem data in a local 0-db instance.
- **[0-db](https://github.com/threefoldtech/0-db)** — Acts as a local append-only cache database. All read and write operations from 0-db-fs access this local instance.
- **[0-stor](https://github.com/threefoldtech/0-stor_v2)** — Listens to 0-db events via hooks. When a local segment file reaches its size limit and is closed, 0-stor chunks, encrypts, and uploads it to remote storage locations using forward error correction. If a read request targets an offloaded segment, 0-stor downloads and reconstructs it from the remote backends.

Because 0-db is append-only, the local database grows linearly with writes. Closed segment files are eventually deleted based on retention policy. Access to deleted segments triggers automatic retrieval and reconstruction from remote backends.

## ZOS / Zero-OS

ZOS, also known as Zero-OS, is the operating system layer used to run and manage nodes. It provides the low-level runtime environment for workloads, networking, storage, and automation. QSS Quantum Storage is deployed as part of the storage subsystem on ZOS nodes.

## Relation to ThreeFold

This technology is used within the ThreeFold ecosystem and was first deployed on the ThreeFold Grid. The component itself is designed as reusable infrastructure technology and should be understood by its technical function first, independent of any specific deployment.

## Ownership

This repository is owned and maintained by TF-Tech NV, a Belgian company responsible for the development and maintenance of this technology.

## How to use it

See the [docs](https://github.com/threefoldtech/quantum-storage/tree/master/docs) in this repository for detailed system documentation and usage instructions.

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.
