#!/bin/bash

# We need this to run non interactively. Otherwise we'll be prompted for it
export PULUMI_CONFIG_PASSPHRASE=""

echo -e "\n===== Writing data, copying data to QSFS, and checking hashes ====="

ipv6=$(pulumi stack -s test | grep pub_ipv6 | tr -s " " | cut -d ' ' -f 3 | cut -d '/' -f 1)

ssh -t root@$ipv6 /root/test-scripts/write_data.sh
ssh -t root@$ipv6 /root/test-scripts/copy_to_qsfs.sh
ssh -t root@$ipv6 /root/test-scripts/check_hashes.sh

# Store a copy of the hashes locally, in case we redeploy the VM
scp "root@[$ipv6]:/root/data/md5s_original" ./
