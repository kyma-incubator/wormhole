BIN = wormhole-connector
GOPATH ?= ${GOPATH}
BINDIR ?= ${GOPATH}/bin

.PHONY: all linux aci update-vendor dep clean install

VERSION=$(shell git describe --tags --always --dirty)

build:
	go build -i -o $(BIN) \
		-ldflags "-w -X main.version=$(VERSION)" \
		./main.go

linux:
	# Compile statically linked binary for linux.
	CGO_ENABLED=0 GOOS=linux go build -a -tags -netgo -o $(BIN) \
		-ldflags "-w -X main.version=$(VERSION)" \
		./main.go

aci: linux
	cd aci && \
		sudo ./build.sh

docker: linux
	# NOTE: If you need to push the docker image to a private registry,
	# building like this might not be enough. In that case, run:
	#
	#   docker build -t $$IPADDR_REGISTRY/kinvolk/wormhole-connector .
	docker build -t kinvolk/wormhole-connector .

update-vendor: | dep
	dep ensure
dep:
	@which dep || go get -u github.com/golang/dep/cmd/dep

clean:
	rm -f $(BIN)

install:
	install $(BIN) "$(BINDIR)"
