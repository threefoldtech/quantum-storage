#!/bin/bash

# We need this to run non interactively. Otherwise we'll be prompted for it
export PULUMI_CONFIG_PASSPHRASE=""

echo -e "\n===== Redeploying with vars.new.py and issuing SIGUSR1 ====="

pulumi stack init test

cp vars.new.py vars.py
pulumi up -s test -y --non-interactive

ipv6=$(pulumi stack -s test | grep pub_ipv6 | tr -s " " | cut -d ' ' -f 3 | cut -d '/' -f 1)

# Since we might use this script both in cases where the frontend VM is
# replaced or not, we'll just go ahead and try to issue the SIGUSR1 even though
# it has no effect on a fresh VM
ssh -o StrictHostKeyChecking=accept-new -t root@$ipv6 '
  pkill zstor -SIGUSR1
'
