# These are the new values used in the test

MNEMONIC = ""
NETWORK = "main"

SSH_KEY_PATH = "~/.ssh/id_rsa"

# Node to deploy VM on. Can overlap with Zdb nodes or not, doesn't matter
VM_NODE = 1

# Nodes to deploy Zdbs on
META_NODES = [8, 10, 11, 24]
DATA_NODES = META_NODES

# Size of each data backend Zdb
DATA_SIZE = 1

# Network used to connect to the backend zdbs
# ZDB_CONNECTION = "mycelium"
ZDB_CONNECTION = "ipv6"

# Network used for SSH connection
# SSH_CONNECTION = "mycelium"
SSH_CONNECTION = "ipv6"
