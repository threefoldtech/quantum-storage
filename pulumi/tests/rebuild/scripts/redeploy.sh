#!/bin/bash

# We need this to run non interactively. Otherwise we'll be prompted for it
export PULUMI_CONFIG_PASSPHRASE=""

pulumi stack init test

cp vars.new.py vars.py
pulumi up -s test -y --non-interactive

ipv6=$(pulumi stack -s test | grep pub_ipv6 | tr -s " " | cut -d ' ' -f 3 | cut -d '/' -f 1)

ssh -t root@$ipv6 '
  pkill zstor -SIGUSR1
  # Wait some time to let the rebuild process start. This should be enough?
  sleep 10 
  # Output should show us if any data has been written to the new backends yet
  zstor -c /etc/zstor-default.toml status
'
