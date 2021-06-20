# Copyright IBM Corp All Rights Reserved.
# Copyright London Stock Exchange Group All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#
# -------------------------------------------------------------
# This makefile defines the following targets
#
#   - all (default) - formats the code, downloads vendor libs, and builds executable
#   - fmt - formats the code
#   - vendor - download all third party libraries and puts them inside vendor directory
#   - clean-vendor - removes third party libraries from vendor directory
#   - leaf-hub-status-sync - builds leaf-hub-status-sync as an executable and puts it under build/_output/bin
#   - docker-build - builds docker image locally for running the components using docker
#   - docker-push - pushes the local docker image to docker registry
#   - clean - cleans the build area (all executables under build/bin)
#   - clean-all - superset of 'clean' that also removes vendor dir
#   - lint - runs code analysis tools


.PHONY: all				##formats the code, downloads vendor libs, and builds executable
all: fmt vendor leaf-hub-status-sync

.PHONY: fmt				##format code
fmt:
	@go fmt ./...

.PHONY: vendor			##download all third party libraries and puts them inside vendor directory
vendor:
	@go mod vendor

.PHONY: clean-vendor			##removes third party libraries from vendor directory
clean-vendor:
	-@rm -rf vendor

.PHONY: leaf-hub-status-sync			##builds leaf-hub-status-sync as an executable and puts it under build/bin
leaf-hub-status-sync:
	@go build -o build/bin/leaf-hub-status-sync cmd/manager/main.go

.PHONY: docker-build			##builds docker image locally for running the components using docker
docker-build: all
	@docker build -t leaf-hub-status-sync -f build/Dockerfile .

.PHONY: docker-push			##pushes the local docker image to docker registry
docker-push: docker-build
	@docker tag leaf-hub-status-sync ${IMAGE}
	@docker push ${IMAGE}

.PHONY: clean			##cleans the build area (all executables under build/bin)
clean:
	@rm -rf build/_output/bin

.PHONY: clean-all			##superset of 'clean' that also removes vendor dir
clean-all: clean-vendor clean

.PHONY: lint				##runs code analysis tools
lint:
	go vet ./...
	golangci-lint run ./...

.PHONY: help				##show this help message
help:
	@echo "usage: make [target]\n"; echo "options:"; \fgrep -h "##" $(MAKEFILE_LIST) | fgrep -v fgrep | sed -e 's/\\$$//' | sed -e 's/##//' | sed 's/.PHONY:*//' | sed -e 's/^/  /'; echo "";
