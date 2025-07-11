.PHONY: build docker-build docker-push

BINARY_NAME=quantumd
IMAGE_NAME=quantumd
IMAGE_TAG=latest

build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
	go build -tags netgo -ldflags '-extldflags "-static"' -o $(BINARY_NAME) .

docker-build: build
	docker buildx build -t $(IMAGE_NAME):$(IMAGE_TAG) .

docker-push: docker-build
	docker push $(IMAGE_NAME):$(IMAGE_TAG)
