FROM alpine:latest
ARG ZDB_VERSION=2.0.5
ARG ZDBFS_VERSION=0.1.11
ARG ZSTOR_VERSION=0.4.0
ARG ZINIT_VERSION=0.2.10
RUN apk add fuse3 redis netcat-openbsd coreutils util-linux
RUN wget https://github.com/threefoldtech/0-db/releases/download/v$ZDB_VERSION/zdb-$ZDB_VERSION-linux-amd64-static -O /bin/zdb
RUN wget https://github.com/threefoldtech/0-db-fs/releases/download/v$ZDBFS_VERSION/zdbfs-$ZDBFS_VERSION-amd64-linux-static -O /bin/zdbfs
RUN wget https://github.com/threefoldtech/0-stor_v2/releases/download/v$ZSTOR_VERSION/zstor_v2-x86_64-linux-musl -O /bin/zstor
RUN wget https://github.com/threefoldtech/zinit/releases/download/v$ZINIT_VERSION/zinit -O /sbin/zinit
RUN chmod +x /bin/zdb /bin/zdbfs /bin/zstor /sbin/zinit
RUN mkdir /data/
ADD rootfs .
ENTRYPOINT ["/sbin/zinit", "init", "--container"]
