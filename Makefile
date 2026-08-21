.PHONY: all build test clean install

BINARY_NAME=ate-watch
BINARY_DIR=cmd/ate-watch
BINARY_PATH=$(BINARY_DIR)/$(BINARY_NAME)

all: build

build:
	go build -o $(BINARY_PATH) ./cmd/ate-watch

test:
	go test -v ./...

install:
	go install ./cmd/ate-watch

clean:
	rm -f $(BINARY_PATH) $(BINARY_NAME)
