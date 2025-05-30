FROM alpine:latest

# Set the base path. If `/mnt`, then Zdbfs storage will be used
ENV BASE_PATH=/mnt

RUN apk add --no-cache wget openssl neovim

# Install 0-db and zdbfs
RUN wget https://github.com/threefoldtech/0-db/releases/download/v2.0.8/zdb-2.0.8-linux-amd64-static && \
    wget https://github.com/threefoldtech/0-db-fs/releases/download/v0.1.10/zdbfs-0.1.10-amd64-linux-static && \
    chmod +x zdb* && \
    cp zdb-2.0.8-linux-amd64-static /usr/bin/zdb && \
    cp zdbfs-0.1.10-amd64-linux-static /usr/bin/zdbfs

# Install Garage
RUN wget https://garagehq.deuxfleurs.fr/_releases/v1.1.0/x86_64-unknown-linux-musl/garage && \
    chmod +x garage && \
    cp garage /usr/bin

# Install warp
RUN wget https://github.com/minio/warp/releases/download/v1.1.4/warp_Linux_arm64.tar.gz && \
    tar xf warp_Linux_arm64.tar.gz && \
    cp warp /usr/bin

COPY prep-warp.sh /prep-warp.sh
RUN chmod +x /prep-warp.sh

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]
