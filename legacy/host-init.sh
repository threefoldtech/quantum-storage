#!/bin/sh
mkdir -p /mnt/zdbfs

cd /

if ! nc -z 127.0.0.1 9900; then
    zdb --datasize $((32 * 1024 * 1024)) --mode seq --listen 127.0.0.1 --hook /tmp/zstor-hook.sh --data /zdb/data --index /zdb/index --background
fi

cat << EOF | redis-cli -p 9900
NSNEW zdbfs-meta
NSNEW zdbfs-data
NSNEW zdbfs-temp
NSSET zdbfs-temp password hello
NSSET zdbfs-temp public 0
EOF

zdbfs -o background /mnt/zdbfs

if ! nc -z 127.0.0.1 2379; then
    etcd &
fi

echo "Ready"
