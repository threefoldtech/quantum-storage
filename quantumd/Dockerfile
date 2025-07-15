FROM ubuntu:24.04

# Install wget
RUN apt-get update && apt-get install -y wget redis

# Install zinit
RUN wget -O /sbin/zinit https://github.com/threefoldtech/zinit/releases/download/v0.2.14/zinit && \
    chmod +x /sbin/zinit

# Copy pre-built binary and hook symlink
COPY quantumd /usr/local/bin/quantumd
COPY quantumd-hook /usr/local/bin/quantumd-hook

# Copy default configuration file (for --local use)
COPY config.example.yaml /etc/quantumd/config.yaml

# Create required directories
RUN mkdir -p /data /mnt/qsfs /var/log /etc/zinit

ENTRYPOINT ["/sbin/zinit", "init"]
