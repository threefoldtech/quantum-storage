#!/bin/bash

# This script generates random data files in regular storage on the VM then
# takes the checksum of each one and stores the sums in a file. By using
# regular storage, we get a baseline idea about write performance and also
# ensure that our later data integrity checks compare to something totally
# independent of the QSFS machinery.

echo "===== Creating 10 test files with 100MB random data each ====="
# Create 10 files with 100mb random data
mkdir -p /root/data
for i in {1..10}; do
  echo "Creating file$i.dat..."
  dd if=/dev/urandom of=/root/data/file$i.dat bs=1M count=100
done

echo -e "\n===== Calculating MD5 checksums of source files ====="
# Calculate and MD5 sum for each file and write to file
rm -f /root/data/md5s_original
for i in {1..10}; do
  md5sum /root/data/file$i.dat | cut -d " " -f 1 | tee -a /root/data/md5s_original
done
