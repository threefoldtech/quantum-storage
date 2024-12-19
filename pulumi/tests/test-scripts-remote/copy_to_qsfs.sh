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
ls -lh /data/data/zdbfs-data
for file in /data/data/zdbfs-data/*; do
while true; do
    # Originally we just looked at the exit code of `check` but this was not
    # reliable. We need to wait for a hash output to be sure zstor has finished
    # storing the file
    check_output=$(zstor -c /etc/zstor-default.toml check --file "$file")
    if [ ! -z "$check_output" ]; then
      echo $file $check_output
      break
    fi
    sleep 2
  done
done
