FROM alpine:latest
ARG ZDB_VERSION=2.0.1
ARG ZDBFS_VERSION=0.1.9
ARG ZSTOR_VERSION=0.3.0-rc.9
ARG ZINIT_VERSION=0.2.2
RUN apk add fuse3 redis netcat-openbsd coreutils &&\
    wget https://github.com/threefoldtech/0-db/releases/download/v$ZDB_VERSION/zdb-$ZDB_VERSION-linux-amd64-static -O /bin/zdb &&\
    wget https://github.com/threefoldtech/0-db-fs/releases/download/v$ZDBFS_VERSION/zdbfs-$ZDBFS_VERSION-amd64-linux-static -O /bin/zdbfs &&\
    wget https://github.com/threefoldtech/0-stor_v2/releases/download/v$ZSTOR_VERSION/zstor_v2-x86_64-linux-musl -O /bin/zstor &&\
    wget https://github.com/threefoldtech/zinit/releases/download/v$ZINIT_VERSION/zinit -O /sbin/zinit &&\
    chmod +x /bin/zdb /bin/zdbfs /bin/zstor /sbin/zinit &&\
    mkdir /data/
ADD rootfs .
COPY create_namespaces hook.sh /bin/
ENTRYPOINT ["/sbin/zinit", "init", "--container"]
