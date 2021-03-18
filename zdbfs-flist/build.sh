#/bin/bash
set -ex

prefix="${HOME}/zdbfs-prefix-root"

apt-get update
DEBIAN_FRONTEND=noninteractive \
    apt-get install -y wget build-essential meson libudev-dev udev pkg-config

mkdir -p "${prefix}"
mkdir -p "${prefix}/bin"
mkdir -p "${prefix}/lib"
mkdir -p "${prefix}/var/lib"

pushd "/tmp/"

wget https://github.com/etcd-io/etcd/releases/download/v3.4.14/etcd-v3.4.14-linux-amd64.tar.gz
wget https://github.com/threefoldtech/0-stor_v2/releases/download/v0.2.0-rc.1/zstor_v2-x86_64-linux-musl
wget https://github.com/threefoldtech/0-db/releases/download/v1.3.0-rc1/zdb-1.3.0-rc1-linux-amd64-gnu
wget https://github.com/threefoldtech/0-flist/releases/download/v2.0.1/zflist-2.0.1-amd64-linux-gnu
wget https://github.com/threefoldtech/0-db-fs/releases/download/v0.1.3/zdbfs-0.1.3-amd64-linux-gnu

tar -xvf etcd-v3.4.14-linux-amd64.tar.gz

chmod +x zstor_v2-x86_64-linux-musl
chmod +x zdb-1.3.0-rc1-linux-amd64-gnu
chmod +x zflist-2.0.1-amd64-linux-gnu
chmod +x zdbfs-0.1.3-amd64-linux-gnu

cp zstor_v2-x86_64-linux-musl "${prefix}/bin/zstor-v2"
cp zdb-1.3.0-rc1-linux-amd64-gnu "${prefix}/bin/zdb"
cp zflist-2.0.1-amd64-linux-gnu "${prefix}/bin/zflist"
cp zdbfs-0.1.3-amd64-linux-gnu "${prefix}/bin/zdbfs"
cp etcd-v3.4.14-linux-amd64/etcd "${prefix}/bin/etcd"

popd

libfuse() {
    wget https://github.com/libfuse/libfuse/releases/download/fuse-3.10.2/fuse-3.10.2.tar.xz
    tar -xf fuse-3.10.2.tar.xz

    cd fuse-3.10.2
    mkdir build && cd build
    meson ..
    ninja
    ninja install

    cp /usr/local/lib/x86_64-linux-gnu/libfuse3.so.3.10.2 "${prefix}/lib/libfuse3.so.3.10.2"
    cp /usr/local/bin/fusermount3 "${prefix}/bin/fusermount3"

    pushd "${prefix}/lib"
    ln -s libfuse3.so.3.10.2 libfuse3.so.3
    popd
}

libfuse

# add zdb-hook.sh to

tar -zcvpf /tmp/zdbfs-image.tar.gz -C ${prefix} .

