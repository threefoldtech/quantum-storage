#!/bin/bash

# This script starts up the qsfs, ensuring the mount point exists

set -x

# Primitive idempotency
zinit | grep -q zstor && exit

echo
echo Creating Zdbfs mountpoint
mkdir -p /mnt/qsfs

echo
echo Copying zinit service files
cp /root/zinit/zstor.yaml /etc/zinit
cp /root/zinit/zdb.yaml /etc/zinit
cp /root/zinit/zdbfs.yaml /etc/zinit

echo
echo Starting up zinit services
zinit monitor zstor
zinit monitor zdb
zinit monitor zdbfs

if [ -f /etc/prometheus.yaml ]; then
    echo
    echo Installing Prometheus
    apt install -y prometheus

    echo
    echo Copying Prometheus zinit service files
    cp /root/zinit/node-exporter.yaml /etc/zinit
    cp /root/zinit/prometheus.yaml /etc/zinit

    echo
    echo Starting up Prometheus zinit services
    zinit monitor node-exporter
    zinit monitor prometheus
fi
