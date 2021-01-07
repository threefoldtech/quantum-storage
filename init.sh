#!/bin/sh
zdb --datasize $((32 * 1024 * 1024)) --mode seq --listen 127.0.0.1 --hook /lib/zdb/zstor-hook.sh --background --data /zdb/data --index /zdb/index
nscreate
zdbfs /mnt &
etcd &

sh
