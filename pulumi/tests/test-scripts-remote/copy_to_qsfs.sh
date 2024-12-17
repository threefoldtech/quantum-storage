#!/bin/bash

# This script copies the data files into the QSFS and waits for uploading to
# complete.

echo -e "\n===== Installing pv tool for transfer monitoring ====="
apt update &> /dev/null && apt install -y pv &> /dev/null

echo -e "\n===== Copying files to QSFS mount with progress monitoring ====="
# Copy files to the qsfs mount and check speed
for i in {1..10}; do
    echo "Copying file$i.dat..."
    pv -s 100m "/root/data/file$i.dat" > "/mnt/qsfs/file$i.dat"
done

echo -e "\n===== Waiting for all data files to upload ====="
# At this point, all the data is in zdb data files. Since we set the rotation
# time for 10 seconds, the last data file should get rotated and uploaded via
# zstor without much delay. At that point, a new empty data file will be
# created, which we don't care about
for file in /data/data/zdbfs-data/*; do
  while ! zstor -c /etc/zstor-default.toml check --file "$file" &> /dev/null; do
    sleep 2
  done
  echo $file
done
