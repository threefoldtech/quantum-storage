.PHONY: build docker-build docker-push

BINARY_NAME=quantumd
HOOK_NAME=quantumd-hook
IMAGE_NAME=quantumd
IMAGE_TAG=latest

build:
	go build -o $(BINARY_NAME) .

docker-build: build
	docker buildx build -t $(IMAGE_NAME):$(IMAGE_TAG) .

docker-push: docker-build
	docker push $(IMAGE_NAME):$(IMAGE_TAG)
