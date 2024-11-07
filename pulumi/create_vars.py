def get_list_input(prompt):
    while True:
        try:
            user_input = input(prompt)
            # Convert string input like "1,2,3,4" to list of integers
            numbers = [int(x.strip()) for x in user_input.split(',')]
            return numbers
        except ValueError:
            print("Please enter valid numbers separated by commas (e.g., 1,2,3,4)")

# Get user input
mnemonic = input("Enter your mnemonic: ")
network = input("Enter the network (main/test): ")
vm_node = input("Enter ID of the VM to deploy the QSFS workload: ")

print("\nEnter the IDs of the meta-data nodes (comma-separated numbers, e.g., 1,2,3,4)")
meta_nodes = get_list_input("META_NODES: ")

print("\nEnter the IDs of the data nodes (comma-separated numbers, e.g., 1,2,3,4)")
data_nodes = get_list_input("DATA_NODES: ")

# Create the vars.py content
content = f'''MNEMONIC = "{mnemonic}"
NETWORK = "{network}"

# Node to deploy VM on. Can overlap with Zdb nodes or not, doesn't matter
VM_NODE = {vm_node}

# Nodes to deploy Zdbs on
META_NODES = {meta_nodes}
DATA_NODES = {data_nodes}

# Size of each data backend Zdb
DATA_SIZE = 1

# Network used to connect to the backend zdbs
# ZDB_CONNECTION = "mycelium"
ZDB_CONNECTION = "ipv6"
'''

# Write to vars.py
with open('vars.py', 'w') as f:
    f.write(content)

print("\nvars.py has been created successfully!")