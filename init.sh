#!/bin/sh
apk add strace
zdb --datasize $((32 * 1024 * 1024)) --mode seq --listen 127.0.0.1 --hook /lib/zdb/zstor-hook.sh --data /zdb/data --index /zdb/index & # --background

sleep 2

nscreate
zdbfs /mnt &
etcd &

MINIO_DISK_USAGE_CRAWL_DEBUG=on MINIO_DISK_USAGE_CRAWL_ENABLE=off minio server /mnt &

sh
