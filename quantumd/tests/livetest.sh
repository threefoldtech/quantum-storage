#!/bin/bash
make docker-build
docker rm -f quantumd-test
docker run -d --privileged --env-file .env --name quantumd-test quantumd:latest
docker cp config.yaml quantumd-test:/etc/quantumd.yaml
docker exec -it  quantumd-test bash
