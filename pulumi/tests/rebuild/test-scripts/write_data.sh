#!/bin/bash

# We need this to run non interactively. Otherwise we'll be prompted for it
export PULUMI_CONFIG_PASSPHRASE=""

ipv6=$(pulumi stack -s test | grep pub_ipv6 | tr -s " " | cut -d ' ' -f 3 | cut -d '/' -f 1)

ssh -t root@$ipv6 '
  echo "===== Creating 10 test files with 100MB random data each ====="
  # Create 10 files with 100mb random data
  for i in {1..10}; do
    echo "Creating file$i.dat..."
    dd if=/dev/urandom of=file$i.dat bs=1M count=100
  done
'

ssh -t root@$ipv6 '
  echo -e "\n===== Calculating MD5 checksums of source files ====="
  # Calculate and print MD5 sum for each file
  for i in {1..10}; do
    md5sum file$i.dat
  done
' > md5s_original

ssh -t root@$ipv6 '
  echo -e "\n===== Installing pv tool for transfer monitoring ====="
  apt update &> /dev/null && apt install -y pv &> /dev/null

  echo -e "\n===== Copying files to QSFS mount with progress monitoring ====="
  # Copy files to the qsfs mount and check speed
  for i in {1..10}; do
      echo "Copying file$i.dat..."
      pv -s 100m "file$i.dat" > "/mnt/qsfs/file$i.dat"
  done

  echo -e "\n===== Waiting for all data files to upload ====="
  # Here we are taking advantage of the fact that there is a 10 second delay
  # before the last data file gets rotated, as specified in zinit/zdb.yaml.
  # After the rotation, there will be a new file with a higher index number
  # that is not uploaded to zstor, but we do not care about that file since it
  # will have no data
  for file in /data/data/zdbfs-data/*; do
    while ! zstor -c /etc/zstor-default.toml check "$file"; do
      sleep 2
    done
  done
'
