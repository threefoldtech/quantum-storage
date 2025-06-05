FROM ubuntu:22.04

# Install dependencies, we just need the redis-cli tool from redis
# tree is helpful to visual how the system is working
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    wget \
    ca-certificates \
    redis \
    tree \
    && rm -rf /var/lib/apt/lists/*

# Version parameters
ENV ZDBFS_VERSION="0.1.11"
ENV ZDB_VERSION="2.0.8"
ENV ZSTOR_VERSION="0.5.0-rc.1"
ENV ZINIT_VERSION="0.2.25"

# Download and install binaries
RUN wget -O /usr/local/bin/zdbfs https://github.com/threefoldtech/0-db-fs/releases/download/v${ZDBFS_VERSION}/zdbfs-${ZDBFS_VERSION}-amd64-linux-static && \
    wget -O /usr/local/bin/zdb https://github.com/threefoldtech/0-db/releases/download/v${ZDB_VERSION}/zdb-${ZDB_VERSION}-linux-amd64-static && \
    wget -O /usr/local/bin/zdb-hook.sh https://raw.githubusercontent.com/threefoldtech/quantum-storage/master/lib/zdb-hook.sh && \
    wget -O /bin/zstor https://github.com/threefoldtech/0-stor_v2/releases/download/v${ZSTOR_VERSION}/zstor_v2-x86_64-linux-musl  && \
    wget -O /sbin/zinit https://github.com/threefoldtech/zinit/releases/download/v${ZINIT_VERSION}/zinit-linux-x86_64

# Set executable permissions
RUN chmod +x /usr/local/bin/* /bin/zstor /sbin/zinit

COPY zinit/ /etc/zinit

COPY setup.sh /setup.sh
RUN chmod +x /setup.sh

COPY zstor_config.toml /etc/zstor-default.toml

ENTRYPOINT ["/sbin/zinit", "init"]
