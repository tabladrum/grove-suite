BINARY := relay
BIN_DIR := bin
CMD := ./cmd/relay

.PHONY: build test lint install clean

build:
	go build -o $(BIN_DIR)/$(BINARY) $(CMD)

test:
	go test ./...

lint:
	go vet ./...

install:
	go install $(CMD)

clean:
	rm -rf $(BIN_DIR)
