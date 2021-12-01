FROM alpine:3.13 as builder
ARG ZDB_VERSION=2.0.0
ARG ZDBFS_VERSION=0.1.9
ARG ZSTOR_VERSION=0.3.0-rc.9
ARG ZINIT_VERSION=0.2.2
RUN apk add hiredis-dev linux-headers build-base
COPY check_zstor.c /
COPY create_namespaces.c /
RUN gcc create_namespaces.c -lhiredis -o create_namespaces -static
RUN gcc check_zstor.c -o /check_zstor -static
RUN wget https://github.com/threefoldtech/0-db/releases/download/v$ZDB_VERSION/zdb-$ZDB_VERSION-linux-amd64-static -O /zdb
RUN wget https://github.com/threefoldtech/0-db-fs/releases/download/v$ZDBFS_VERSION/zdbfs-$ZDBFS_VERSION-amd64-linux-static -O /zdbfs
RUN wget https://github.com/threefoldtech/0-stor_v2/releases/download/v$ZSTOR_VERSION/zstor_v2-x86_64-linux-musl -O /zstor
RUN wget https://github.com/threefoldtech/zinit/releases/download/v$ZINIT_VERSION/zinit -O /sbin/zinit
RUN chmod +x /zdb /zdbfs /zstor /sbin/zinit

FROM alpine:latest
RUN apk add fuse3
ADD rootfs .
COPY hook.sh /bin
COPY --from=builder /create_namespaces /check_zstor /zdbfs /zstor /bin/
COPY --from=builder /sbin/zinit /sbin/zinit
ENTRYPOINT ["/sbin/zinit", "init"]
