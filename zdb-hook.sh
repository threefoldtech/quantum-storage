#!/bin/sh
set -ex

action="$1"
instance="$2"
zstorconf="/etc/zstor-default.toml"
zstorbin="/bin/zstor"

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
    exit 0
fi

if [ "$action" == "missing-data" ]; then
    # restore missing data file
    ${zstorbin} -c ${zstorconf} retrieve --file "$3"
    exit $?
fi

# unknown action
exit 1

