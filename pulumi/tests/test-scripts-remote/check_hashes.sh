#!/bin/bash

echo -e "\n===== Calculating MD5 hashes of stored files ====="
# Calculate and MD5 hash for each file and write to new hash file. Overwrite
# the new hashes file since we might run this multiple times
rm -f /root/data/md5s_new
for i in {1..10}; do
  md5sum /mnt/qsfs/file$i.dat | cut -d " " -f 1 >> /root/data/md5s_new
done

echo -e "\n===== Comparing hashes ===="
if cmp -s /root/data/md5s_original /root/data/md5s_new; then
  echo -e "\n===== Hashes match, success ====="
else
  echo -e "\n===== Hashes differ, failure ===="
fi
