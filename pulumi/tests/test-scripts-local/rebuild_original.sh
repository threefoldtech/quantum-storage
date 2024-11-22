#!/bin/bash

# We need this to run non interactively. Otherwise we'll be prompted for it
export PULUMI_CONFIG_PASSPHRASE=""

echo -e "\n===== Removing local data files and reconstructin from backends ====="

ipv6=$(pulumi stack -s test | grep pub_ipv6 | tr -s " " | cut -d ' ' -f 3 | cut -d '/' -f 1)

ssh -t root@$ipv6 'rm /data/data/zdbfs-data/*'
ssh -t root@$ipv6 /root/test-scripts/check_hashes.sh
