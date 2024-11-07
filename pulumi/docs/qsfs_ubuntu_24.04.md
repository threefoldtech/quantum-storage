<h1>QSFS Micro VM on the TFGrid</h1>

<h2>Table of Contents</h2>

- [Introduction](#introduction)
- [Prequisites](#prequisites)
- [Steps](#steps)

---

## Introduction

We present how to deploy QSFS workloads on the TFGrid with a micro VM.

## Prequisites

- Micro VM on TFGrid with Ubuntu 24.04, IPv6 and Mycelium
- SSH into the VM

## Steps

- Simply paste the script on the CLI and follow the instructions

```
#!/bin/bash

# This script is written for a Ubuntu 24.04 environment
# It works out of the box with a micro VM with ipv6 and Mycelium on the TFGrid.

# Install prerequisites
apt update
apt install -y curl
curl -fsSL https://get.pulumi.com | sh
export PATH=$PATH:/root/.pulumi/bin
apt install python3 python3-pip python3-venv git -y

# Set Python
python3 -m venv .venv
source .venv/bin/activate
pip3 install pulumi pulumi_random pulumi_threefold==0.6.10 

# Clone the quantum-storage repository
git clone https://github.com/threefoldtech/quantum-storage
cd quantum-storage
git checkout development_pulumi_scripts
cd pulumi

# Create SSH keypair to access the QSFS nodes
ssh-keygen -t rsa -N "" -f ~/.ssh/id_rsa
chmod 700 ~/.ssh
chmod 600 ~/.ssh/id_rsa
chmod 644 ~/.ssh/id_rsa.pub

# Start Pulumi
pulumi login --local
pulumi up
```