#!/bin/bash

# This script installs all binaries and scripts needed for QSFS. It doesn't actually start up the services though

set -x

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

if ! [ -f /usr/local/bin/retry-uploads.sh ]; then
    wget -O /usr/local/bin/retry-uploads.sh https://raw.githubusercontent.com/threefoldtech/quantum-storage/master/lib/retry-uploads.sh
fi

if ! [ -f /bin/zstor ]; then
    wget -O /bin/zstor https://github.com/threefoldtech/0-stor_v2/releases/download/v0.4.0/zstor_v2-x86_64-linux-musl
fi

echo
echo Setting permissions for downloaded binaries
chmod +x /usr/local/bin/* /bin/zstor

if [ -f /etc/prometheus.yaml ]; then
    echo
    echo Installing Prometheus
    apt update
    apt install -y prometheus prometheus-pushgateway curl
fi
