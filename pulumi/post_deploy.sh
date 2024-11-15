#!/bin/bash

set -x

# Primitive idempotency
zinit | grep -q zstor && exit

# Grab binaries and hook script. Make sure that all are executable
# We check first if the files exist, to support testing other builds by
# uploading them into the VM before running this script
if ! [ -f /usr/local/bin/zdbfs ]; then
    wget -O /usr/local/bin/zdbfs https://github.com/threefoldtech/0-db-fs/releases/download/v0.1.11/zdbfs-0.1.11-amd64-linux-static
fi

if ! [ -f /usr/local/bin/zdb ]; then
    wget -O /usr/local/bin/zdb https://github.com/threefoldtech/0-db/releases/download/v2.0.8/zdb-2.0.8-linux-amd64-static
fi

if ! [ -f /usr/local/bin/zdb-hook.sh ]; then
    wget -O /usr/local/bin/zdb-hook.sh https://raw.githubusercontent.com/threefoldtech/quantum-storage/master/lib/zdb-hook.sh
fi

if ! [ -f /bin/zstor ]; then
    wget -O /bin/zstor https://github.com/threefoldtech/0-stor_v2/releases/download/v0.4.0/zstor_v2-x86_64-linux-musl
fi

echo
echo Setting permissions for downloaded binaries
chmod +x /usr/local/bin/* /bin/zstor

echo
echo Creating Zdbfs mountpoint
mkdir -p /mnt/qsfs

echo
echo Starting up zinit services
zinit monitor zstor
zinit monitor zdb
zinit monitor zdbfs

# If user didn't supply a prometheus config, then give the files a non "yaml"
# extension so zinit doesn't start them if the VM reboots
if [ -f /etc/prometheus.yaml ]; then
    apt install -y prometheus
    zinit monitor node-exporter
    zinit monitor prometheus
else
    mv /etc/zinit/prometheus.yaml /etc/zinit/prometheus.yaml.deactivated
    mv /etc/zinit/node-exporter.yaml /etc/zinit/node-exporter.yaml.deactivated
fi

# Zdbfs will fail on first attempt because zdb isn't ready yet (could add a
# test to zdb to fix this, maybe using redis-cli, nc, or ss)
sleep 1
zinit
