Running an S3 compatible storage service on top of QSFS is a natural use case. Here we have some scaffolding for benchmarking the performance of open source S3 solutions using QSFS as the underlying storage.

For this purpose, it's enough to just run Zdbfs. We are interested in the best case performance when all data is part of the local cache. Retrieving offloaded data will of course affect performance, but that's not specific to the S3 solution itself.

The high level looks like this:

1. Spin up a Docker container that mounts a Zdbfs and launches the S3 solution
2. Run the `warp` benchmark tool from the MinIO project
3. We can target either the Zdbfs storage or raw disk storage, to compare
