#!/bin/bash
make docker-build
docker rm -f quantumd-test && docker run -d --privileged --name quantumd-test quantumd:latest
docker cp config.yaml quantumd-test:/etc/quantumd.yaml
docker exec -it --env-file .env quantumd-test bash
