#!/bin/bash

# We need this to run non interactively. Otherwise we'll be prompted for it
export PULUMI_CONFIG_PASSPHRASE=""

echo -e "\n===== Removing local data files and reconstructin from backends ====="

ipv6=$(pulumi stack -s test | grep pub_ipv6 | tr -s " " | cut -d ' ' -f 3 | cut -d '/' -f 1)

ssh -t root@$ipv6 '
  rm /data/data/zdbfs-data/*
  for i in {1..10}; do
    md5sum file$i.dat
  done
' > md5s_new

diff md5_original md5_new

if cmp -s md5_original md5_new; then
  echo -e "\n===== Hashes match after rebuild, success ====="
else
  echo -e "\n===== Hashes differ after rebuild, failure ===="
