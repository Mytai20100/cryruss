BINARY   := cryruss
VERSION  := 0.1v
GOFLAGS  := -trimpath
LDFLAGS  := -s -w -X 'github.com/cryruss/cryruss.Version=$(VERSION)'
BUILD_DIR := ./dist

.PHONY: all build install clean test fmt vet

all: build

build:
	mkdir -p $(BUILD_DIR)
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY) ./cmd/cryruss

install: build
	cp $(BUILD_DIR)/$(BINARY) $(GOPATH)/bin/$(BINARY)

install-user: build
	mkdir -p $(HOME)/.local/bin
	cp $(BUILD_DIR)/$(BINARY) $(HOME)/.local/bin/$(BINARY)

clean:
	rm -rf $(BUILD_DIR)

test:
	go test ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

run-serve:
	$(BUILD_DIR)/$(BINARY) serve

.DEFAULT_GOAL := build
