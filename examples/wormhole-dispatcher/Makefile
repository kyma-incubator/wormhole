BIN = wormhole-dispatcher

.PHONY: all linux docker

all: docker

linux:
	# Compile statically linked binary for linux.
	CGO_ENABLED=0 GOOS=linux go build -a -tags -netgo -o $(BIN) \
		./main.go

docker: linux
	docker build -t kyma-incubator/wormhole-dispatcher .
