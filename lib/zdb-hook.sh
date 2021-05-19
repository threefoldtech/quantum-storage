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
    namespace=$(basename $(dirname $3))
    if [ "${namespace}" == "zdbfs-temp" ]; then
        # skipping temporary namespace
        exit 0
    fi

    tmpdir=$(mktemp -p /tmp -d zdb.hook.XXXXXXXX.tmp)
    dirbase=$(dirname $3)

    # upload dirty index files
    for dirty in $5; do
        file=$(printf "zdb-index-%05d" $dirty)
        cp ${dirbase}/${file} ${tmpdir}/
    done

    ${zstorbin} -c ${zstorconf} store -s -d -f ${tmpdir} -k ${dirbase} &

    exit 0
fi

if [ "$action" == "jump-data" ]; then
    namespace=$(basename $(dirname $3))
    if [ "${namespace}" == "zdbfs-temp" ]; then
        # skipping temporary namespace
        exit 0
    fi

    # backup data file
    ${zstorbin} -c ${zstorconf} store -s --file "$3"

    exit 0
fi

if [ "$action" == "missing-data" ]; then
    # restore missing data file
    ${zstorbin} -c ${zstorconf} retrieve --file "$3"
    exit $?
fi

# unknown action
exit 1
