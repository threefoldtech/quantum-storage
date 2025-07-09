FROM alpine:3.18

# Install zinit
RUN wget -O /sbin/zinit https://github.com/threefoldtech/zinit/releases/download/v0.2.14/zinit && \
    chmod +x /sbin/zinit

# Install redis-cli
RUN apk add --no-cache redis

# Copy pre-built binary
COPY quantumd /usr/local/bin/quantumd

# Create required directories
RUN mkdir -p /data /mnt/qsfs /var/log /etc/zinit

ENTRYPOINT ["/sbin/zinit", "init"]
