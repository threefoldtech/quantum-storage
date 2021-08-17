#/bin/bash
set -ex

# Ubuntu 20.04

prefix="${HOME}/zdbfs-prefix-root"

apt-get update
DEBIAN_FRONTEND=noninteractive apt-get install -y wget fuse3

mkdir -p "${prefix}"
mkdir -p "${prefix}/bin"
mkdir -p "${prefix}/lib"
mkdir -p "${prefix}/var/lib"

cp ../lib/zdb-hook.sh "${prefix}/var/lib/"

pushd "/tmp/"

zstor_version="0.3.0-rc.5"
zdb_version="2.0.0-rc5"
zdbfs_version="0.1.7"
zflist_version="2.0.1"
zdbctl_version="0.0.4"

wget https://github.com/threefoldtech/0-stor_v2/releases/download/v${zstor_version}/zstor_v2-x86_64-linux-musl
wget https://github.com/threefoldtech/0-db/releases/download/v${zdb_version}/zdb-${zdb_version}-linux-amd64-static
wget https://github.com/threefoldtech/0-flist/releases/download/v${zflist_version}/zflist-${zflist_version}-amd64-linux-gnu
wget https://github.com/threefoldtech/0-db-fs/releases/download/v${zdbfs_version}/zdbfs-${zdbfs_version}-amd64-linux-static
wget https://github.com/threefoldtech/quantum-storage/releases/download/v${zdbctl_version}/zdbctl

chmod +x zstor_v2-x86_64-linux-musl
chmod +x zdb-${zdb_version}-linux-amd64-static
chmod +x zflist-${zflist_version}-amd64-linux-gnu
chmod +x zdbfs-${zdbfs_version}-amd64-linux-static
chmod +x zdbctl

cp -v zstor_v2-x86_64-linux-musl "${prefix}/bin/zstor-v2"
cp -v zdb-${zdb_version}-linux-amd64-static "${prefix}/bin/zdb"
cp -v zflist-${zflist_version}-amd64-linux-gnu "${prefix}/bin/zflist"
cp -v zdbfs-${zdbfs_version}-amd64-linux-static "${prefix}/bin/zdbfs"
cp -v $(which fusermount3) "${prefix}/bin/fusermount3"
cp -v zdbctl "${prefix}/bin/zdbctl"

popd

tar -zcvpf /tmp/zdbfs-image.tar.gz -C ${prefix} .

