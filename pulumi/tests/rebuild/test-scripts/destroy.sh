#!/bin/bash

# We need this to run non interactively. Otherwise we'll be prompted for it
export PULUMI_CONFIG_PASSPHRASE=""

pulumi down -s test -y --non-interactive
pulumi stack rm -yf test
