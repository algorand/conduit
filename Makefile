SRCPATH		:= $(shell pwd)
export GOPATH := $(shell go env GOPATH)
GOPATH1 := $(firstword $(subst :, ,$(GOPATH)))

# pinned filename can be overridden in CI with an env variable.
CI_E2E_FILENAME ?= f99e7b0c/rel-nightly

GOLDFLAGS += -X github.com/algorand/conduit/version.Hash=$(shell git log -n 1 --pretty="%H")
GOLDFLAGS += -X github.com/algorand/conduit/version.ShortHash=$(shell git log -n 1 --pretty="%h")
GOLDFLAGS += -X github.com/algorand/conduit/version.CompileTime=$(shell date -u +%Y-%m-%dT%H:%M:%S%z)
GOLDFLAGS += -X "github.com/algorand/conduit/version.ReleaseVersion=Dev Build"

COVERPKG := $(shell go list ./...  | grep -v '/cmd/' | egrep -v '(testing|test|mocks)$$' |  paste -s -d, - )

# Used for e2e test
export GO_IMAGE = golang:$(shell go version | cut -d ' ' -f 3 | tail -c +3 )

# This is the default target, build everything:
all: conduit

conduit:
	go generate ./... && cd cmd/conduit && go build -ldflags='${GOLDFLAGS}'

install:
	cd cmd/conduit && go install -ldflags='${GOLDFLAGS}'

e2e-conduit: conduit
	export PATH=$(GOPATH1)/bin:$(PATH); pip3 install e2e_tests/ && e2econduit --s3-source-net $(CI_E2E_FILENAME) --conduit-bin cmd/conduit/conduit

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

.PHONY: all conduit check test lint fmt e2e-conduit
