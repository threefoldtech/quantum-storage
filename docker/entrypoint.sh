#!/bin/sh

# Start backend zdbs for zstor. Frontend zdb is started later
zdb --port 9901 --data /data/data1 --index /data/index1 --background --logfile /var/log/zdb1.log
zdb --port 9902 --data /data/data2 --index /data/index2 --background --logfile /var/log/zdb2.log
zdb --port 9903 --data /data/data3 --index /data/index3 --background --logfile /var/log/zdb3.log
zdb --port 9904 --data /data/data4 --index /data/index4 --background --logfile /var/log/zdb4.log

# Create namespaces for backends
redis-cli -p 9901 NSNEW data1
redis-cli -p 9901 NSSET data1 password zdbpassword
redis-cli -p 9901 NSSET data1 mode seq

redis-cli -p 9901 NSNEW meta1
redis-cli -p 9901 NSSET meta1 password zdbpassword

redis-cli -p 9902 NSNEW data2
redis-cli -p 9902 NSSET data2 password zdbpassword
redis-cli -p 9902 NSSET data2 mode seq

redis-cli -p 9902 NSNEW meta2
redis-cli -p 9902 NSSET meta2 password zdbpassword

redis-cli -p 9903 NSNEW data3
redis-cli -p 9903 NSSET data3 password zdbpassword
redis-cli -p 9903 NSSET data3 mode seq

redis-cli -p 9903 NSNEW meta3
redis-cli -p 9903 NSSET meta3 password zdbpassword

redis-cli -p 9904 NSNEW data4
redis-cli -p 9904 NSSET data4 password zdbpassword
redis-cli -p 9904 NSSET data4 mode seq

redis-cli -p 9904 NSNEW meta4
redis-cli -p 9904 NSSET meta4 password zdbpassword

# Start zstor. The config file location is hardcoded in the hook script,
# so we must use this config path.
zstor -c /etc/zstor-default.toml --log_file /var/log/zstor.log monitor &

# Start frontend zdb for zdbfs. The hook script wlil try to run zstor from the
# start, so we should have it ready first.
zdb \
  --index /data/index \
  --data /data/data \
  --logfile /var/log/zdb.log \
  --datasize 67108864 \
  --hook /usr/local/bin/zdb-hook.sh \
  --rotate 900 \
  --background

# Start zdbfs, it will connect to the zdb on port 9900 by default
zdbfs -o autons -o background /mnt > /var/log/zdbfs.log 2>&1 &

# Keep container alive
tail -f /dev/null
