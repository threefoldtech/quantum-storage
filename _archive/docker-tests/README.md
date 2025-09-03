This Docker based QSFS deployment is meant strictly for testing and instructional purposes. Running the Dockerfile will create an entire QSFS in a single container, with four backend Zdb instances. Since each backend is its own instance with its own port number, it's possible to simulate network interruptions using firewall rules to block the respective individual ports, for example.

## Usage

First, build the container image:

```
docker buildx build -t qsfs .
```

Then launch an instance of the container. Enhanced capabilities are required to use FUSE:

```
docker run -d --cap-add SYS_ADMIN --device /dev/fuse --name qsfs qsfs
```

Now you can `exec` a shell in the container and try writing some data into the QSFS mount (at `/mnt`):

```
docker exec -it qsfs bash

# Inside the container now...
# Write 1gb of data into the QSFS
dd if=/dev/random of=/mnt/random bs=1M count=1000

# We can visualize how data was written to the frontend and backends with tree
tree -h /data
```
