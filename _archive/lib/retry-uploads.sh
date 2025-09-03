#!/bin/bash

# This script is intended to supplement the zdb hook script for a QSFS
# deployment. It performs the same function, of ensuring that data and index
# files get uploaded via zstor, but it works without an explicit trigger from
# zdb about which files are ready to be stored. This is needed because so far
# there is no retry when the original actions fail, such as due to zstor being
# unavailable or if too many backends are down.
#
# Since zdb data files are immutable, we keep a list of which ones we've checked
# to save check operations in the future. Indexes on the other hand are mutable,
# so we check these everytime. A more advanced implementation might have the zdb
# hook script write the dirty index paths somewhere that this script would then
# clear out after a double check (actually on second look, zdb provides commands
# to check dirty index files manually, so we could use that). A zstor check
# operation can take ~0.5s with mediocre latency to the metadata backends and
# possibly longer with bad latency, competing traffic, etc. This is probably
# okay up to some hundreds of index files but may become a problem eventually
#
# The default behavior is to run in a loop with a ten minute sleep after each
# pass. The sleep time can be passed as an optional argument, and a negative
# sleep time results in only a single run.

# Ten minutes
DEFAULT_SLEEP_TIME=600

UPLOADED_DATA_FILES_PATH=/data/uploaded_data_files

if [ -n "$1" ]; then
    sleep_time=$1
else
    sleep_time=$DEFAULT_SLEEP_TIME
fi

# Function to check and upload file if needed
check_and_upload_file() {
    local file="$1"

    # Skip if file doesn't exist
    [ ! -f "$file" ] && return

    # Get remote and local hashes
    local remote_hash=$(zstor -c /etc/zstor-default.toml check --file "$file")
    local local_hash=$(b2sum -l 128 "$file" | cut -d' ' -f1)

    # Store file if hashes don't match or remote check failed
    if [ -z "$remote_hash" ] || [ "$remote_hash" != "$local_hash" ]; then
        zstor -c /etc/zstor-default.toml store --file "$file"
        elif [ -n "$remote_hash" ] && [[ "$file" == /data/data/* ]]; then
            echo "$file $local_hash" >> "$UPLOADED_DATA_FILES_PATH"
    fi
}

while true; do
    # Process each type of file (data and index) for both zdbfs-data and zdbfs-meta
    # Create temp dir for index files
    tmpdir=$(mktemp -d)
    for namespace in "zdbfs-data" "zdbfs-meta"; do
        mkdir $tmpdir/$namespace
        # Check and upload namespace file
        namespace_file="/data/index/$namespace/zdb-namespace"
        check_and_upload_file "$namespace_file"

        # Process data and index files
        for type in "data" "index"; do
            # The index directory also has the namespace file, so we exclude
            # that by only looking for files starting with d or i
            path_base=/data/$type/$namespace/${type:0:1}

            # We want to check every file except for the largest sequence
            # number, so we sort and throw away the last row. Here an ls even
            # without -1 helps sort to work, while echo does't. Not sure why
            for file in $(ls -1 $path_base* | sort -V | head -n -1); do
                # Since index files are mutable, we freeze them in a tmp folder
                # to avoid any issues with concurrent reads/writes
                if [ "$type" = "index" ]; then
                    tmp_path="$tmpdir/$namespace/$(basename $file)"
                    cp "$file" "$tmp_path"
                    check_and_upload_file "$tmp_path"
                elif ! grep -q "$file" "$UPLOADED_DATA_FILES_PATH" 2>/dev/null; then
                    check_and_upload_file "$file"
                fi
            done
        done
    done
    rm -rf "$tmpdir"

    # If we have a Prometheus push gateway running, send metrics to it
    if pgrep prometheus-push > /dev/null; then
        timestamp=$(date +%s)
        cat <<EOF | curl -s --data-binary @- localhost:9091/metrics/job/qsfs_retry_uploads
# TYPE last_run_time gauge
last_run_time $timestamp
EOF
    fi

    if ((sleep_time < 0)); then
        break
    else
        sleep $sleep_time
    fi
done
