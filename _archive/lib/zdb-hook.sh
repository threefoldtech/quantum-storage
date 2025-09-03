#!/bin/sh
set -ex

prefix="${ZDBFS_PREFIX:-/}"
action="$1"
instance="$2"
zstorconf="${prefix}/etc/zstor-default.toml"
zstorbin="${prefix}/bin/zstor"
zstorindex="${prefix}/data/index"
zstordata="${prefix}/data/data"

if [ "$action" = "close" ]; then
    for namespace in `ls $zstordata`; do
        if [ "${namespace}" = "zdbfs-temp" ]; then
            continue
        fi
        indexdir="$zstorindex/$namespace"
        datadir="$zstordata/$namespace"
        lastactive="`ls $indexdir | grep -v zdb-namespace | cut -d'i' -f2 | sort -n | tail -n 1`"
        datafile="$datadir/d$lastactive"
        indexfile="$indexdir/i$lastactive"
        ${zstorbin} -c ${zstorconf} store -s --file "$datafile"
        ${zstorbin} -c ${zstorconf} store -s --file "$indexfile"
    done
    exit 0
fi

if [ "$action" = "ready" ]; then
    ${zstorbin} -c ${zstorconf} test
    exit $?
fi

if [ "$action" = "namespace-created" ] || [ "$action" = "namespace-updated" ]; then
    if [ "$3" = "zdbfs-temp" ]; then
        # skipping temporary namespace
        exit 0
    fi
    file="$zstorindex/$3/zdb-namespace"

    # backup zdb-namespace file
    ${zstorbin} -c ${zstorconf} store -s --file "$file"

    exit 0
fi

if [ "$action" = "jump-index" ]; then
    
    namespace=$(basename $(dirname $3))
    if [ "${namespace}" = "zdbfs-temp" ]; then
        # skipping temporary namespace
        exit 0
    fi

    tmpdir=$(mktemp -p /tmp -d zdb.hook.tmp.XXXXXXXX)
    dirbase=$(dirname $3)

    # upload dirty index files
    for dirty in $5; do
        file=$(printf "i%d" $dirty)
        cp ${dirbase}/${file} ${tmpdir}/
    done
    cp "$3" ${tmpdir}/
    nice ${zstorbin} -c ${zstorconf} store -s -d -f ${tmpdir} -k ${dirbase} &

    exit 0
fi

if [ "$action" = "jump-data" ]; then
    namespace=$(basename $(dirname $3))
    if [ "${namespace}" = "zdbfs-temp" ]; then
        # skipping temporary namespace
        exit 0
    fi

    # backup data file
    nice ${zstorbin} -c ${zstorconf} store -s --file "$3"

    exit 0
fi

if [ "$action" = "missing-data" ]; then
    # restore missing data file
    ${zstorbin} -c ${zstorconf} retrieve --file "$3"
    exit $?
fi

# unknown action
exit 1
