#!/usr/bin/env bash
set -ex

prefix="${ZDBFS_PREFIX:-/}"
action="$1"
instance="$2"
zstorconf="${prefix}/etc/zstor-default.toml"
zstorbin="${prefix}/bin/zstor"

if [ "$action" == "ready" ]; then
    ${zstorbin} -c ${zstorconf} test
    exit $?
fi

if [ "$action" == "jump-index" ]; then
    # skip index saving, file are mutable
    # ${zstorbin} -c ${zstorconf} store --file "$3"
    exit 0
fi

if [ "$action" == "jump-data" ]; then
    namespace=$(basename $(dirname $3))
    if [ "${namespace}" == "zdbfs-temp" ]; then
        # skipping temporary namespace
        exit 0
    fi

    # backup data file
    ${zstorbin} -c ${zstorconf} store --file "$3"

    if [ $? == 0 ]; then
        # echo "File saved, cleanup local file: $3"
        # rm -f "$3"
        echo "Skipping deletion"
    fi

    exit 0
fi

if [ "$action" == "missing-data" ]; then
    # restore missing data file
    ${zstorbin} -c ${zstorconf} retrieve --file "$3"
    exit $?
fi

# unknown action
exit 1
