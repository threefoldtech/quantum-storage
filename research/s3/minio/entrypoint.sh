#!/bin/sh

# Start 0-db
zdb > /var/log/zdb.log 2>&1 &

sleep 1

# Start 0-db-fs
zdbfs -o autons -o background /mnt > /var/log/zdbfs.log 2>&1 &

sleep 1

# Create dirs for garage
mkdir -p ${BASE_PATH}/data
mkdir -p ${BASE_PATH}/meta

# Start Garage
minio server > /var/log/minio.log 2>&1 &

sleep 1

# Creates key and bucket
/prep-warp.sh $BASE_PATH

# Keep container alive
tail -f /dev/null
