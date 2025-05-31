FROM alpine:latest

RUN apk add --no-cache wget

# Install 0-db and zdbfs
RUN wget https://github.com/threefoldtech/0-db/releases/download/v2.0.8/zdb-2.0.8-linux-amd64-static && \
    wget https://github.com/threefoldtech/0-db-fs/releases/download/v0.1.10/zdbfs-0.1.10-amd64-linux-static && \
    chmod +x zdb* && \
    mv zdb-2.0.8-linux-amd64-static /usr/bin/zdb && \
    mv zdbfs-0.1.10-amd64-linux-static /usr/bin/zdbfs

# Install Minio and client
RUN wget https://dl.min.io/server/minio/release/linux-amd64/minio && \
    wget https://dl.min.io/client/mc/release/linux-amd64/mc && \
    chmod +x minio && \
    chmod +x mc && \
    mv minio /usr/bin && \
    mv mc /usr/bin

# Install warp
RUN wget https://github.com/minio/warp/releases/download/v1.1.4/warp_Linux_arm64.tar.gz && \
    tar xf warp_Linux_arm64.tar.gz && \
    mv warp /usr/bin


# Where Minio will store its data. If we use /mnt, then Zdbfs storage will be
# used, otherwise we use raw SSD.
ENV MINIO_VOLUMES="/mnt/data"
ENV MINIO_ROOT_USER=admin
ENV MINIO_ROOT_PASSWORD=secret-admin

COPY prep-warp.sh /prep-warp.sh
RUN chmod +x /prep-warp.sh

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]
