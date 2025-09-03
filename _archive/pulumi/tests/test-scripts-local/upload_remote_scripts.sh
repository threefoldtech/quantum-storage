#!/bin/bash

# We need this to run non interactively. Otherwise we'll be prompted for it
export PULUMI_CONFIG_PASSPHRASE=""

# Use first argument if provided, otherwise default to 'test'
STACK_NAME=${1:-test}
ipv6=$(pulumi stack -s $STACK_NAME | grep pub_ipv6 | tr -s " " | cut -d ' ' -f 3 | cut -d '/' -f 1)

# Get directory of script file. This way the path to  upload is always correct
# regardless of where the script is run from
SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

echo -e "\n===== Copying remote test scripts to the VM ===="
scp -o StrictHostKeyChecking=accept-new -r "$SCRIPT_DIR"/../test-scripts-remote/ "root@[$ipv6]:/root/test-scripts"
