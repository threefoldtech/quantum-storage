FROM ubuntu:22.04

# Install dependencies
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    wget \
    curl \
    prometheus \
    prometheus-pushgateway \
    && rm -rf /var/lib/apt/lists/*

# Download and install binaries
RUN wget -O /usr/local/bin/zdbfs https://github.com/threefoldtech/0-db-fs/releases/download/v0.1.11/zdbfs-0.1.11-amd64-linux-static && \
    wget -O /usr/local/bin/zdb https://github.com/threefoldtech/0-db/releases/download/v2.0.8/zdb-2.0.8-linux-amd64-static && \
    wget -O /usr/local/bin/zdb-hook.sh https://raw.githubusercontent.com/threefoldtech/quantum-storage/master/lib/zdb-hook.sh && \
    wget -O /usr/local/bin/retry-uploads.sh https://raw.githubusercontent.com/threefoldtech/quantum-storage/master/lib/retry-uploads.sh && \
    wget -O /bin/zstor https://github.com/threefoldtech/0-stor_v2/releases/download/v0.4.0/zstor_v2-x86_64-linux-musl

# Set executable permissions
RUN chmod +x /usr/local/bin/* /bin/zstor

# Copy prometheus config if it exists (optional)
COPY prometheus.yaml /etc/prometheus.yaml

# Default command
CMD ["/bin/bash"]
