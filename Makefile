SRCPATH		:= $(shell pwd)
export GOPATH := $(shell go env GOPATH)
GOPATH1 := $(firstword $(subst :, ,$(GOPATH)))

# TODO: ensure any additions here are mirrored in misc/release.py
GOLDFLAGS += -X github.com/algorand/indexer/version.Hash=$(shell git log -n 1 --pretty="%H")
GOLDFLAGS += -X github.com/algorand/indexer/version.Dirty=$(if $(filter $(strip $(shell git status --porcelain|wc -c)), "0"),,true)
GOLDFLAGS += -X github.com/algorand/indexer/version.CompileTime=$(shell date -u +%Y-%m-%dT%H:%M:%S%z)
GOLDFLAGS += -X github.com/algorand/indexer/version.GitDecorateBase64=$(shell git log -n 1 --pretty="%D"|base64|tr -d ' \n')
#GOLDFLAGS += -X github.com/algorand/indexer/version.ReleaseVersion=$(shell cat .version)

COVERPKG := $(shell go list ./...  | grep -v '/cmd/' | egrep -v '(testing|test|mocks)$$' |  paste -s -d, - )

# Used for e2e test
export GO_IMAGE = golang:$(shell go version | cut -d ' ' -f 3 | tail -c +3 )

# This is the default target, build everything:
all: conduit

conduit:
	go generate ./... && cd cmd/conduit && go build -ldflags="${GOLDFLAGS}"

# check that all packages (except tests) compile
check:
	go build ./...

test:
	go test -coverpkg=$(COVERPKG) ./... -coverprofile=coverage.txt -covermode=atomic ${TEST_FLAG}

lint:
	golangci-lint run -c .golangci.yml
	go vet ./...

fmt:
	go fmt ./...

.PHONY: all conduit check test lint fmt
