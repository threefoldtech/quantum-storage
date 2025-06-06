#!/bin/bash

# This script is for recovering an existing QSFS onto a new VM

create_mount_point() {
    echo
    echo "Creating QSFS mount point at /mnt/qsfs..."
    mkdir -p /mnt/qsfs
}

start_services() {
    echo
    echo "Starting zstor and zdb services..."
    cp /root/zinit/* /etc/zinit
    zinit monitor zstor
    zinit monitor zdb
}

install_redis() {
    echo
    echo "Installing redis-cli..."
    apt update && apt install -y redis
}

setup_temp_namespace() {
    echo
    echo "Setting up temp namespace..."
    redis-cli -p 9900 NSNEW zdbfs-temp
    redis-cli -p 9900 NSSET zdbfs-temp password hello
    redis-cli -p 9900 NSSET zdbfs-temp public 0
    redis-cli -p 9900 NSSET zdbfs-temp mode seq
}

recover_metadata() {
    echo
    echo "Recovering metadata indexes..."
    zstor -c /etc/zstor-default.toml retrieve --file /data/index/zdbfs-meta/zdb-namespace
    i=0
    while true; do
        result=$(zstor -c /etc/zstor-default.toml retrieve --file /data/index/zdbfs-meta/i$i 2>&1)
        if echo $result | grep -q error
            then break
        fi
        i=$((i+1))
    done

    echo
    echo "Retrieving latest metadata data file..."
    last_meta_index=$(ls /data/index/zdbfs-meta | tr -d i | sort -n | tail -n 1)
    zstor -c /etc/zstor-default.toml retrieve --file /data/data/zdbfs-meta/d$last_meta_index
}

recover_data() {
    echo
    echo "Recovering data indexes..."
    zstor -c /etc/zstor-default.toml retrieve --file /data/index/zdbfs-data/zdb-namespace
    i=0
    while true; do
        result=$(zstor -c /etc/zstor-default.toml retrieve --file /data/index/zdbfs-data/i$i 2>&1)
        if echo $result | grep -q error
            then break
        fi
        i=$((i+1))
    done

    echo
    echo "Retrieving latest data data file..."
    last_data_index=$(ls /data/index/zdbfs-data | tr -d i | sort -n | tail -n 1)
    zstor -c /etc/zstor-default.toml retrieve --file /data/data/zdbfs-data/d$last_data_index
}

start_zdbfs() {
    echo
    echo "Starting ZDBFS service..."
    zinit monitor zdbfs
}

# Main execution if not sourced
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    create_mount_point
    start_services
    install_redis
    setup_temp_namespace
    recover_metadata
    recover_data
    start_zdbfs
fi
