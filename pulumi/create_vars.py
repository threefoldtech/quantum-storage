def get_list_input(prompt, default_value):
    while True:
        try:
            user_input = input(f"{prompt} (press Enter for default: {default_value}): ")
            if user_input.strip() == "":
                return default_value
            # Convert string input like "1,2,3,4" to list of integers
            numbers = [int(x.strip()) for x in user_input.split(',')]
            return numbers
        except ValueError:
            print("Please enter valid numbers separated by commas (e.g., 1,2,3,4)")

def get_str_input(prompt, default_value=None):
    if default_value is None:
        # For required fields (no default)
        while True:
            user_input = input(prompt).strip()
            if user_input:
                return user_input
            print("This field is required. Please enter a value.")
    else:
        # For fields with defaults
        user_input = input(f"{prompt} (press Enter for default: {default_value}): ").strip()
        return user_input if user_input else default_value

# Default values
DEFAULT_NETWORK = "main"
DEFAULT_VM_NODE = 5
DEFAULT_META_NODES = [1, 3, 5, 8]
DEFAULT_DATA_NODES = [1, 3, 5, 8]
DEFAULT_DATA_SIZE = 1
DEFAULT_ZDB_CONNECTION = "ipv6"

# Get user input
mnemonic = get_str_input("Enter your mnemonic: ")  # No default value
network = get_str_input("Enter the network (main/test)", DEFAULT_NETWORK)
vm_node = get_str_input("Enter ID of the VM to deploy the QSFS workload", DEFAULT_VM_NODE)

print("\nEnter the IDs of the meta-data nodes (comma-separated numbers)")
meta_nodes = get_list_input("META_NODES", DEFAULT_META_NODES)

print("\nEnter the IDs of the data nodes (comma-separated numbers)")
data_nodes = get_list_input("DATA_NODES", DEFAULT_DATA_NODES)

# Create the vars.py content
content = f'''MNEMONIC = "{mnemonic}"
NETWORK = "{network}"

# Node to deploy VM on. Can overlap with Zdb nodes or not, doesn't matter
VM_NODE = {vm_node}

# Nodes to deploy Zdbs on
META_NODES = {meta_nodes}
DATA_NODES = {data_nodes}

# Size of each data backend Zdb
DATA_SIZE = {DEFAULT_DATA_SIZE}

# Network used to connect to the backend zdbs
# ZDB_CONNECTION = "mycelium"
ZDB_CONNECTION = "{DEFAULT_ZDB_CONNECTION}"
'''

# Write to vars.py
with open('vars.py', 'w') as f:
    f.write(content)

print("\nvars.py has been created successfully!")