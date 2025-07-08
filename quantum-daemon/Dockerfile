FROM alpine:3.18

# Install zinit
RUN wget -O /sbin/zinit https://github.com/threefoldtech/zinit/releases/download/v0.2.25/zinit-linux-x86_64 && \
    chmod +x /sbin/zinit

# Copy pre-built binary
COPY quantumd /usr/local/bin/quantumd

# Copy zinit services
COPY assets/zinit/* /etc/zinit/

# Create required directories
RUN mkdir -p /data /mnt/qsfs /var/log

ENTRYPOINT ["/sbin/zinit", "init"]
