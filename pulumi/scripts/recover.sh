#!/bin/bash

# Start zstor
cp /root/zinit/zstor.yaml /etc/zinit
zinit monitor zstor

# Recover the (empty) temp namespace
# zstor -c /etc/zstor-default.toml retrieve --file /data/index/zdbfs-temp/zdb-namespace
# zstor -c /etc/zstor-default.toml retrieve --file /data/index/zdbfs-temp/i0
# zstor -c /etc/zstor-default.toml retrieve --file /data/data/zdbfs-temp/d0

# Can't remember why I was trying to recover the temp namespace
# I think the hook ignores it anyway, so we can just start fresh

apt update && apt install redis
redis-cli -p 9900 NSNEW zdbfs-temp
redis-cli -p 9900 NSSET zdbfs-temp password hello
redis-cli -p 9900 NSSET zdbfs-temp public 0
redis-cli -p 9900 NSSET zdbfs-temp mode seq

# Recover meta data index and (empty) working data file
zstor -c /etc/zstor-default.toml retrieve --file /data/index/zdbfs-meta/zdb-namespace
i=0
while true; do
  result=$(zstor -c /etc/zstor-default.toml retrieve --file /data/index/zdbfs-meta/i$i 2>&1)
  if echo $result | grep -q error
    then break
  fi
  i=$((i+1))
done

last_meta_index=$(ls /data/index/zdbfs-meta | tr -d i | sort -n | tail -n 1)
zstor -c /etc/zstor-default.toml retrieve --file /data/data/zdbfs-meta/d$last_meta_index

# Recover data index and (empty) working file
zstor -c /etc/zstor-default.toml retrieve --file /data/index/zdbfs-data/zdb-namespace
i=0
while true; do
  result=$(zstor -c /etc/zstor-default.toml retrieve --file /data/index/zdbfs-data/i$i 2>&1)
  if echo $result | grep -q error
    then break
  fi
  i=$((i+1))
done

last_data_index=$(ls /data/index/zdbfs-data | tr -d i | sort -n | tail -n 1)
zstor -c /etc/zstor-default.toml retrieve --file /data/data/zdbfs-data/d$last_data_index


# Start zdb and zdbfs

cp /root/zinit/* /etc/zinit
zinit monitor zdb
zinit monitor zdbfs
