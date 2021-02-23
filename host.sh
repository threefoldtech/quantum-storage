#!/bin/bash
set -ex

devpath=$(dirname $0)
prefix=/usr/local

apt-get update
apt-get install -y libfuse3-dev libhiredis-dev build-essential git curl redis-tools netcat

cp zdb-hook.sh /tmp/zstor-hook.sh
chmod +x /tmp/zstor-hook.sh

# fix ash -> bash
sed 's#bin/sh#bin/bash#g' -i /tmp/zstor-hook.sh

cd /tmp

rm -rf 0-db
rm -rf 0-db-fs

git clone --depth=1 https://github.com/threefoldtech/0-db
git clone --depth=1 https://github.com/threefoldtech/0-db-fs
curl -L https://github.com/etcd-io/etcd/releases/download/v3.4.14/etcd-v3.4.14-linux-amd64.tar.gz > etcd-v3.4.14-linux-amd64.tar.gz
curl -L https://github.com/threefoldtech/0-stor_v2/releases/download/v0.1.1/zstor_v2-x86_64-linux-musl > zstor_v2-x86_64-linux-gnu

pushd 0-db/libzdb && make release && popd
pushd 0-db/zdbd && make release && popd
pushd 0-db-fs && make release && popd

tar -xf etcd-v3.4.14-linux-amd64.tar.gz

cp -v 0-db/zdbd/zdb $prefix/bin/
cp -v 0-db-fs/zdbfs $prefix/bin/
cp -v etcd-v3.4.14-linux-amd64/etcd $prefix/bin/
cp -v zstor_v2-x86_64-linux-gnu $prefix/bin/zstor
chmod +x $prefix/bin/zstor

