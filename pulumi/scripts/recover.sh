#!/bin/bash

# This script is for recovering an existing QSFS onto a new VM

echo
echo "Creating QSFS mount point at /mnt/qsfs..."
mkdir -p /mnt/qsfs

echo
echo "Starting zstor and zdb services..."
cp /root/zinit/* /etc/zinit
zinit monitor zstor
zinit monitor zdb

# The temp namespace is not backed up, so we just create it manually
echo
echo "Installing redis-cli..."
apt update && apt install -y redis

echo
echo "Setting up temp namespace..."
redis-cli -p 9900 NSNEW zdbfs-temp
redis-cli -p 9900 NSSET zdbfs-temp password hello
redis-cli -p 9900 NSSET zdbfs-temp public 0
redis-cli -p 9900 NSSET zdbfs-temp mode seq

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


# Start zdbfs
echo
echo "Starting ZDBFS service..."
zinit monitor zdbfs
