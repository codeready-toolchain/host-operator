# It's necessary to set this because some environments don't link sh -> bash.
SHELL := /bin/bash

include ./make/verbose.mk
.DEFAULT_GOAL := help
include ./make/help.mk
include ./make/out.mk
include ./make/find-tools.mk
include ./make/go.mk
include ./make/git.mk
include ./make/dev.mk
include ./make/format.mk
include ./make/lint.mk
include ./make/test.mk
# include ./make/docker.mk

.PHONY: build
## Build the operator
build: ./out/operator

.PHONY: clean
clean:
	$(Q)-rm -rf ${V_FLAG} ./out
	$(Q)-rm -rf ${V_FLAG} ./vendor
	$(Q)-rm -rf ${V_FLAG} ./tmp
	$(Q)go clean ${X_FLAG} ./...

./out/operator: $(shell find . -path ./vendor -prune -o -name '*.go' -print)
	#$(Q)operator-sdk generate k8s
	$(Q)CGO_ENABLED=0 GOARCH=amd64 GOOS=linux \
		go build ${V_FLAG} \
		-ldflags "-X ${GO_PACKAGE_PATH}/cmd/manager.Commit=${GIT_COMMIT_ID} -X ${GO_PACKAGE_PATH}/cmd/manager.BuildTime=${BUILD_TIME}" \
		-o ./out/operator \
		cmd/manager/main.go

.PHONY: update-olm-catalog
## Update CRD manifests in deploy/{operator-name}/{csv-version} the using latest API's 
## Run this goal as `make csv-version=0.1.1 update-olm-catalog`
update-olm-catalog:
	$(Q)operator-sdk olm-catalog gen-csv --update-crds --csv-version=$(csv-version)
