#!/bin/sh
set -x

echo args "$@"

action="$1"
instance="$2"
zstorconf="/data/zstor.toml"
zstorbin="/bin/zstor"
zstorindex="/data/index"
zstordata="/data/data"

restore_namespace () {
    if [ "$1" = "zdbfs-temp" ]; then
        return
    fi
    indexdir="$zstorindex/$1"
    datadir="$zstordata/$1"
    ci=0 # current index
    
    ${zstorbin} -c ${zstorconf} retrieve --file "$indexdir/zdb-namespace"
    while true; do
        indexpath="$indexdir/i$ci"
        ${zstorbin} -c ${zstorconf} retrieve --file "$indexpath"
        if [ $? != 0 ]; then
            if [ $ci != 0 ]; then
                datapath="$datadir/d$(( $ci - 1))"
                ${zstorbin} -c ${zstorconf} retrieve --file "$datapath"
            fi
            break
        fi
        ci=$(( $ci + 1 ))
    done
}

if [ "$action" = "ready" ]; then
    exit 0
fi

# doesn't work because close is not the last hook
# if [ "$action" = "close" ]; then
#     cat /proc/*/comm 2> /dev/null | grep '[h]ook.sh'
#     while ! cat /proc/*/comm | grep '[h]ook.sh' | wc -l | grep -q '^1$'; do
#         # there's another hook other than ourself
#         sleep .1
#     done
#     echo 'all hooks are dead'
#     exit 0
# fi

if [ "$action" = "namespaces-init" ]; then
    restore_namespace zdbfs-meta
    restore_namespace zdbfs-data
    exit 0
fi
# zdbfs-data /data/index/zdbfs-data/i18 /data/data/zdbfs-data/d18 17 18
if [ "$action" = "namespace-closing" ]; then
    namespace="$3"
    ci="$4"
    cd="$5"
    dirty="$6"
    if [ "${namespace}" = "zdbfs-temp" ]; then
        continue
    fi
    indexdir=$(dirname $ci)
    datadir=$(dirname $cd)
    lastindex="$indexdir/$ci"
    lastdata="$datadir/$cd"

    ${zstorbin} -c ${zstorconf} store -s --file "$ci"
    ${zstorbin} -c ${zstorconf} store -s --file "$cd"
    
    for index in $dirty; do
        indexfile="$indexdir/i$index"
        ${zstorbin} -c ${zstorconf} store -s --file "$indexfile"
    done
    exit 0
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
    ${zstorbin} -c ${zstorconf} store -s -d -f ${tmpdir} -k ${dirbase} &

    exit 0
fi

if [ "$action" = "jump-data" ]; then
    namespace=$(basename $(dirname $3))
    if [ "${namespace}" = "zdbfs-temp" ]; then
        # skipping temporary namespace
        exit 0
    fi

    # backup data file
    ${zstorbin} -c ${zstorconf} store -s --file "$3"

    exit 0
fi

if [ "$action" = "missing-data" ]; then
    # restore missing data file
    ${zstorbin} -c ${zstorconf} retrieve --file "$3"
    exit $?
fi

# unknown action
exit 1
