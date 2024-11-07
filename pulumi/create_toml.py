def get_int_input(prompt, default_value):
    while True:
        user_input = input(f"{prompt} (press Enter for default: {default_value}): ").strip()
        if user_input == "":
            return default_value
        try:
            return int(user_input)
        except ValueError:
            print("Please enter a valid number")

def get_str_input(prompt, default_value):
    user_input = input(f"{prompt} (press Enter for default: {default_value}): ").strip()
    return user_input if user_input else default_value

# Get user input with default values
minimal_shards = get_int_input("Enter minimal shards", 2)
expected_shards = get_int_input("Enter expected shards", 4)
redundant_groups = get_int_input("Enter redundant groups", 0)
redundant_nodes = get_int_input("Enter redundant nodes", 0)
zdbfs_mountpoint = get_str_input("Enter ZDBFS mountpoint", "/mnt/qsfs")

# Create the TOML content
content = f'''minimal_shards = {minimal_shards}
expected_shards = {expected_shards}
redundant_groups = {redundant_groups}
redundant_nodes = {redundant_nodes}
root = "/"
zdbfs_mountpoint = "{zdbfs_mountpoint}"
socket = "/tmp/zstor.sock"
prometheus_port = 9100
zdb_data_dir_path = "/data/data/zdbfs-data/"
max_zdb_data_dir_size = 2560

[compression]
algorithm = "snappy"

[meta]
type = "zdb"

[meta.config]
prefix = "zstor-meta"
'''

# Write to zstor_config.toml
with open('zstor_config.base.toml', 'w') as f:
    f.write(content)

print("\nzstor_config.toml has been created successfully!")