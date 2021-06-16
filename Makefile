# Copyright IBM Corp All Rights Reserved.
# Copyright London Stock Exchange Group All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#
# -------------------------------------------------------------
# This makefile defines the following targets
#
#   - all (default) - performs code formatting and builds the code
#   - fmt - format code
#   - bsa_example - builds bsa_example as an executable
#   - test - performs tests
#   - lint - runs code analysis tools
#   - clean - cleans the build directories


.PHONY: all				##perform code formatting and builds the code
all: fmt build

.PHONY: fmt				##format code
fmt:
	@go fmt ./...

.PHONY: build		##build the controller
build:
	@go build -o build/_output/bin/leaf-hub-status-sync cmd/manager/main.go

build/test:
	@mkdir -p build/test

.PHONY: test				##perform tests
test: lint build/test
	go test -o build/test/bsa_test -c ./pkg/bsa
	@DYLD_LIBRARY_PATH=${BSA_C_DIRECTORY}/bsa/Debug ./build/test/bsa_test -test.v

.PHONY: clean			##clean the build directories
clean:
	@rm -rf build/text

.PHONY: lint				##runs code analysis tools
lint:
	go vet ./...
	golangci-lint run ./...

.PHONY: help				##show this help message
help:
	@echo "usage: make [target]\n"; echo "options:"; \fgrep -h "##" $(MAKEFILE_LIST) | fgrep -v fgrep | sed -e 's/\\$$//' | sed -e 's/##//' | sed 's/.PHONY:*//' | sed -e 's/^/  /'; echo "";
