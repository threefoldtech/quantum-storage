#!/bin/bash

# This script waits for all files to be uploaded to zstor (the ones that are
# expected to be uploaded, anyway).

echo -e "\n===== Waiting for all data files to upload ====="

wait_for_upload() {
    while [ -z $(zstor -c /etc/zstor-default.toml check --file "$1") ]; do
        sleep 2
    done
    echo $1
}

for namespace in "zdbfs-data" "zdbfs-meta"; do
    namespace_file="/data/index/$namespace/zdb-namespace"
    if [ -f "$namespace_file" ]; then
        wait_for_upload $namespace_file
    else
        echo Namespace file missing: $namespace_file
    fi

    for type in "data" "index"; do
        # The index directory also has the namespace file, so we exclude that by
        # only looking for files starting with d or i
        path_base=/data/$type/$namespace/${type:0:1}
        # We want to check every file except for the largest sequence number, so
        # we sort and throw away the last row. Here an ls even without -1 helps
        # sort to work, while echo does't. Not sure why
        for file in $(ls -1 $path_base* | sort -V | head -n -1); do
            wait_for_upload $file
        done
    done
done
