#!/bin/bash

# We need this to run non interactively. Otherwise we'll be prompted for it
export PULUMI_CONFIG_PASSPHRASE=""

# If we fail to delete the deployment, we should keep the stack around
pulumi down -s test -y --non-interactive && pulumi stack rm -yf test
