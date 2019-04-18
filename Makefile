VERSION := $(shell git describe --tags)
BUILD := $(shell git rev-parse --short HEAD)
PROJECTNAME := $(shell basename $(PWD))

# Go related variables.
export GO111MODULE=on
GOBASE := $(shell pwd)
GOPATH := $(GOBASE)/vendor:$(GOBASE)
GOBIN := $(GOBASE)/bin
GOFILES = $(shell find . -type f -name '*.go' -not -path "./vendor/*")

# Use linker flags to provide version/build settings
LDFLAGS=-ldflags "-X=main.Version=$(VERSION) -X=main.Build=$(BUILD)"
STATIC_FLAGS=-ldflags '-w -extldflags "-static"'



build: $(GOFILES)
	@echo "  >  Building binary..."
	CGO_ENABLED=0 GOPATH=$(GOPATH) GOBIN=$(GOBIN) go build $(STATIC_FLAGS) $(LDFLAGS)  -o $(GOBIN)/$(PROJECTNAME) $(GOFILES)

install:
	@GOPATH=$(GOPATH) GOBIN=$(GOBIN) go install $(GOFILES)

fmt:
	@gofmt -l -w $(GOFILES)

clean:
	@echo "  >  Cleaning build cache"
	@GOPATH=$(GOPATH) GOBIN=$(GOBIN) go clean
	@rm -rf $(GOBIN)

.PONY: clean fmt help
