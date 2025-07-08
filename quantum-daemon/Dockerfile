FROM alpine:3.18 as builder

# Install build dependencies
RUN apk add --no-cache \
    git \
    make \
    go \
    musl-dev

WORKDIR /src
COPY . .

# Build statically linked binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -tags netgo -ldflags '-extldflags "-static"' -o /quantumd .

# Final image
FROM alpine:3.18

# Install zinit
RUN wget -O /sbin/zinit https://github.com/threefoldtech/zinit/releases/download/v0.2.25/zinit-linux-x86_64 && \
    chmod +x /sbin/zinit

# Copy built binary
COPY --from=builder /quantumd /usr/local/bin/quantumd

# Copy zinit services
COPY assets/zinit/* /etc/zinit/

# Create required directories
RUN mkdir -p /data /mnt/qsfs /var/log

ENTRYPOINT ["/sbin/zinit", "init"]
