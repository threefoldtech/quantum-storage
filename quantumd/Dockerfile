FROM ubuntu:24.04

# Install wget
RUN apt-get update && apt-get install -y wget redis tree
# Version parameters
ENV ZDBFS_VERSION="0.1.11"
ENV ZDB_VERSION="2.0.8"
ENV ZSTOR_VERSION="0.5.0-rc.1"
ENV ZINIT_VERSION="0.2.14"

# Download and install binaries. quantumd can do this, but for multiple tests
# it's more efficient to put them in the image. The installer feature can be
# tested by deleting these files
RUN wget -O /usr/local/bin/zdbfs https://github.com/threefoldtech/0-db-fs/releases/download/v${ZDBFS_VERSION}/zdbfs-${ZDBFS_VERSION}-amd64-linux-static && \
    wget -O /usr/local/bin/zdb https://github.com/threefoldtech/0-db/releases/download/v${ZDB_VERSION}/zdb-${ZDB_VERSION}-linux-amd64-static && \
    wget -O /usr/local/bin/zdb-hook.sh https://raw.githubusercontent.com/threefoldtech/quantum-storage/master/lib/zdb-hook.sh && \
    wget -O /bin/zstor https://github.com/threefoldtech/0-stor_v2/releases/download/v${ZSTOR_VERSION}/zstor_v2-x86_64-linux-musl  && \
    wget -O /sbin/zinit https://github.com/threefoldtech/zinit/releases/download/v${ZINIT_VERSION}/zinit

# Set executable permissions
RUN chmod +x /usr/local/bin/* /bin/zstor /sbin/zinit

# Copy pre-built binary and hook symlink
COPY quantumd /usr/local/bin/quantumd
COPY quantumd-hook /usr/local/bin/quantumd-hook

# Copy default configuration file (for --local use)
COPY config.example.yaml /etc/quantumd.yaml

# Create required directories
RUN mkdir -p /data /mnt/qsfs /var/log /etc/zinit

ENTRYPOINT ["/sbin/zinit", "init"]
