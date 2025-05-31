Running an S3 compatible storage service on top of QSFS is a natural use case. Here we have some scaffolding for benchmarking the performance of open source S3 solutions using QSFS as the underlying storage.

For this purpose, it's enough to just run Zdbfs. We are interested in the best case performance when all data is part of the local cache. Retrieving offloaded data will of course affect performance, but that's not specific to the S3 solution itself.

The high level looks like this:

1. Spin up a Docker container that mounts a Zdbfs and launches the S3 solution
2. Run the `warp` benchmark tool from the MinIO project
3. We can target either the Zdbfs storage or raw disk storage, to compare

### Example run

```
cd garage

docker buildx build -t zdbfs-garage .

# We use privileged to allow FUSE for Zdbfs
# The base path of /mnt puts Garage data and metadata on the Zdbfs
docker run -d --privileged --name zdbfs-garage -e BASE_PATH=/mnt zdbfs-garage:latest

# Now we can jump into the container
docker exec -it zdbfs-garage ash

# This script contains an example of running warp in "mixed mode"
# It can be used as a reference to run other modes
/run_warp_benchmark.sh

# After the benchmark, a report is printed and a file with results is written
ls

# We can copy the results file out of the container to save for later
exit # exit the container
docker cp zdbfs-garage:/warp-mixed-2025-05-29[224101]-DH4z.json.zst ./ # example file name

# Stop and remove the container
docker stop zdbfs-garage
docker rm zdbfs-garage

# Now we can start a new container with raw disk backing Garage (use some path outside of /mnt)
docker run -d --privileged --name zdbfs-garage -e BASE_PATH=/tmp zdbfs-garage:latest

# Repeat the benchmark, copy the results file if desired, and remove the container again
```
