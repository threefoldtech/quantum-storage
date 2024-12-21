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
