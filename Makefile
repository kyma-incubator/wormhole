BIN = wormhole-connector
GOPATH ?= ${GOPATH}
BINDIR ?= ${GOPATH}/bin

.PHONY: all clean dep install

VERSION=$(shell git describe --tags --always --dirty)

all:
	go build -i -o $(BIN) \
		-ldflags "-X main.version=$(VERSION)" \
		./main.go

update-vendor: | dep
	dep ensure
dep:
	@which dep || go get -u github.com/golang/dep/cmd/dep

clean:
	rm -f $(BIN)

install:
	install $(BIN) "$(BINDIR)"
