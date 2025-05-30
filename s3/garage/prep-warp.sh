#!/bin/sh

BASE_PATH=$1

garage layout assign -z dc1 -c 1G $(garage status | tail -n 1 | cut -d ' ' -f 1)

garage layout apply --version 1

garage bucket create warp-benchmark-bucket

garage key create warp

garage bucket allow \
  --read \
  --write \
  --owner \
  warp-benchmark-bucket \
  --key warp

KEY_INFO=$(garage key info --show-secret warp)
KEY_ID=$(echo "$KEY_INFO" | grep "Key ID:" | awk '{print $3}')
SECRET_KEY=$(echo "$KEY_INFO" | grep "Secret key:" | awk '{print $3}')

cat > run_warp_benchmark.sh << EOF
#!/bin/sh

warp mixed --host=127.0.0.1:3900 --access-key="$KEY_ID" --secret-key="$SECRET_KEY"
EOF

chmod +x run_warp_benchmark.sh
