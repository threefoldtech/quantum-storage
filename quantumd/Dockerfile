FROM alpine:3.18

# Install zinit
RUN wget -O /sbin/zinit https://github.com/threefoldtech/zinit/releases/download/v0.2.14/zinit && \
    chmod +x /sbin/zinit

# Install redis-cli
RUN apk add --no-cache redis

# Copy pre-built binary and hook symlink
COPY quantumd /usr/local/bin/quantumd
COPY quantumd-hook /usr/local/bin/quantumd-hook

# Copy default configuration file (for --local use)
COPY config.example.yaml /etc/quantumd/config.yaml

# Create required directories
RUN mkdir -p /data /mnt/qsfs /var/log /etc/zinit

ENTRYPOINT ["/sbin/zinit", "init"]
