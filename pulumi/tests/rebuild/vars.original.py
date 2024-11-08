# These are the original values used in the test

MNEMONIC = ""
NETWORK = "test"
# Public SSH key. If empty, we'll attempt to read it from ~/.ssh/*.pub
SSH_KEY = ""

# Node to deploy VM on. Can overlap with Zdb nodes or not, doesn't matter
VM_NODE = 5

# Nodes to deploy Zdbs on
META_NODES = [1, 2, 3, 5]
DATA_NODES = META_NODES

# Size of each data backend Zdb
DATA_SIZE = 1

# Network used to connect to the backend zdbs
# ZDB_CONNECTION = "mycelium"
ZDB_CONNECTION = "ipv6"
