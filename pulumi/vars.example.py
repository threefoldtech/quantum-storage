MNEMONIC = "your words here"
NETWORK = "test"

# Node to deploy VM on. Can overlap with Zdb nodes or not, doesn't matter
VM_NODE = 5

# Nodes to deploy Zdbs on
META_NODES = [1, 3, 5, 8]
DATA_NODES = [1, 3, 5, 8]

# Size of each data backend Zdb
DATA_SIZE = 1

# Network used to connect to the backend zdbs
# ZDB_CONNECTION = "mycelium"
ZDB_CONNECTION = "ipv6"
