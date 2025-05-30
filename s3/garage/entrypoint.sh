#!/bin/sh

# Create garage config

cat > /etc/garage.toml <<EOF
metadata_dir = "${BASE_PATH}/meta"
data_dir = "${BASE_PATH}/data"
db_engine = "sqlite"

replication_factor = 1

rpc_bind_addr = "[::]:3901"
rpc_public_addr = "127.0.0.1:3901"
rpc_secret = "$(openssl rand -hex 32)"

[s3_api]
s3_region = "garage"
api_bind_addr = "[::]:3900"
root_domain = ".s3.garage.localhost"

[s3_web]
bind_addr = "[::]:3902"
root_domain = ".web.garage.localhost"
index = "index.html"

[k2v_api]
api_bind_addr = "[::]:3904"

[admin]
api_bind_addr = "[::]:3903"
admin_token = "$(openssl rand -base64 32)"
metrics_token = "$(openssl rand -base64 32)"
EOF

# Start 0-db
zdb > /var/log/zdb.log 2>&1 &

sleep 1

# Start 0-db-fs
zdbfs -o autons -o background /mnt > /var/log/zdbfs.log 2>&1 &

sleep 1

# Create dirs for garage
mkdir -p ${BASE_PATH}/data
mkdir -p ${BASE_PATH}/meta

# Start Garage
garage server > /var/log/garage.log 2>&1 &

sleep 1

# Creates key and bucket
/prep-warp.sh $BASE_PATH

# Keep container alive
tail -f /dev/null
