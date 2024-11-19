# These can also be specified as env vars with the same names
MNEMONIC = "your words here"
NETWORK = "test"

# In order to run commands on the deployed VM, we need both the public and
# private key files available. This takes the path to the private key file, and
# there should be a matching .pub file
SSH_KEY_PATH = "~/.ssh/id_rsa"

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

# Network used for SSH connection
# SSH_CONNECTION = "mycelium"
SSH_CONNECTION = "ipv6"
