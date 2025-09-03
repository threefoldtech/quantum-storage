#!/bin/bash

# We need this to run non interactively. Otherwise we'll be prompted for it
export PULUMI_CONFIG_PASSPHRASE=""

pulumi stack init test

cp vars.original.py vars.py
pulumi up -s test -y --non-interactive
