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
# Here we are taking advantage of the fact that there is a 10 second delay
# before the last data file gets rotated, as specified in zinit/zdb.yaml.
# After the rotation, there will be a new file with a higher index number
# that is not uploaded to zstor, but we do not care about that file since it
# will have no data
for file in /data/data/zdbfs-data/*; do
  while ! zstor -c /etc/zstor-default.toml check --file "$file" &> /dev/null; do
    sleep 2
  done
done
