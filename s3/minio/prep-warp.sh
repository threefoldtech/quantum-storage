#!/bin/sh

BASE_PATH=$1

mc alias set warp http://127.0.0.1:9000 admin secret-admin

mc mb warp/warp-benchmark-bucket

mc admin user add warp warp secret-warp
mc admin policy attach warp readwrite --user warp
mc admin policy attach warp consoleAdmin --user warp

cat > run_warp_benchmark.sh << EOF
#!/bin/sh

warp mixed --host=127.0.0.1:9000 --access-key=warp --secret-key=secret-warp
EOF

chmod +x run_warp_benchmark.sh
