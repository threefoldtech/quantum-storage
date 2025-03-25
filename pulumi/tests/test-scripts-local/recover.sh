#!/bin/bash

# We need this to run non interactively. Otherwise we'll be prompted for it
export PULUMI_CONFIG_PASSPHRASE=""

echo -e "\n===== Running recover script on remote VM ====="

ipv6=$(pulumi stack -s test | grep pub_ipv6 | tr -s " " | cut -d ' ' -f 3 | cut -d '/' -f 1)

# The scripts uploaded by Pulumi won't be executable. We could fix that elsewhere
ssh -t root@$ipv6 bash /root/scripts/recover.sh
ssh -t root@$ipv6 mkdir /root/data
scp ./md5s_original "root@[$ipv6]:/root/data/md5s_original"
