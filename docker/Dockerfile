FROM ubuntu:22.04

# Install dependencies
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    wget \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Version parameters
ENV ZDBFS_VERSION="0.1.11" \
    ZDB_VERSION="2.0.8" \
    ZSTOR_VERSION="0.4.0" \
    QSFS_REPO="threefoldtech/quantum-storage"

# Download and install binaries
RUN wget -O /usr/local/bin/zdbfs https://github.com/threefoldtech/0-db-fs/releases/download/v${ZDBFS_VERSION}/zdbfs-${ZDBFS_VERSION}-amd64-linux-static && \
    wget -O /usr/local/bin/zdb https://github.com/threefoldtech/0-db/releases/download/v${ZDB_VERSION}/zdb-${ZDB_VERSION}-linux-amd64-static && \
    wget -O /usr/local/bin/zdb-hook.sh https://raw.githubusercontent.com/${QSFS_REPO}/master/lib/zdb-hook.sh && \
    wget -O /bin/zstor https://github.com/threefoldtech/0-stor_v2/releases/download/v${ZSTOR_VERSION}/zstor_v2-x86_64-linux-musl

# Set executable permissions
RUN chmod +x /usr/local/bin/* /bin/zstor

# Default command
CMD ["/bin/bash"]
