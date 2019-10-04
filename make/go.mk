# By default the project should be build under GOPATH/src/github.com/<orgname>/<reponame>
GO_PACKAGE_ORG_NAME ?= $(shell basename $$(dirname $$PWD))
GO_PACKAGE_REPO_NAME ?= $(shell basename $$PWD)
GO_PACKAGE_PATH ?= github.com/${GO_PACKAGE_ORG_NAME}/${GO_PACKAGE_REPO_NAME}

GO111MODULE?=on
export GO111MODULE

.PHONY: build
## Build the operator
build: generate $(OUT_DIR)/operator

$(OUT_DIR)/operator:
	$(Q)CGO_ENABLED=0 GOARCH=amd64 GOOS=linux \
		go build ${V_FLAG} \
		-ldflags "-X ${GO_PACKAGE_PATH}/cmd/manager.Commit=${GIT_COMMIT_ID} -X ${GO_PACKAGE_PATH}/cmd/manager.BuildTime=${BUILD_TIME}" \
		-o $(OUT_DIR)/bin/host-operator \
		cmd/manager/main.go

.PHONY: vendor
vendor:
	$(Q)go mod vendor

NSTEMPLATES_DIR=deploy/templates/nstemplatetiers
NSTEMPLATES_TEST_DIR=test/templates/nstemplatetiers

.PHONY: generate
generate: generate-metadata generate-assets 

.PHONY: generate-assets
generate-assets:
	@echo "generating templates bindata..."
	@go install github.com/go-bindata/go-bindata/...
	@$(GOPATH)/bin/go-bindata -pkg nstemplatetiers -o ./pkg/templates/nstemplatetiers/nstemplatetier_assets.go -nocompress -prefix $(NSTEMPLATES_DIR) $(NSTEMPLATES_DIR)
	@echo "generating test templates bindata..."
	@$(GOPATH)/bin/go-bindata -pkg nstemplatetiers_test -o ./test/templates/nstemplatetiers/nstemplatetier_assets.go -nocompress -prefix $(NSTEMPLATES_TEST_DIR) $(NSTEMPLATES_TEST_DIR)

.PHONY: generate-metadata
generate-metadata: clean-metadata
	@echo "generating namespace templates metadata for manifests in $(NSTEMPLATES_DIR)" 
	@echo "yaml files: $(shell ls $(NSTEMPLATES_DIR)/*.yaml)"
	@echo "yaml files: $(wildcard $(NSTEMPLATES_DIR)/*.yaml)"
	@$(foreach tmpl,$(wildcard $(NSTEMPLATES_DIR)/*.yaml),$(call git_commit,$(tmpl),$(NSTEMPLATES_DIR)/metadata.yaml);)

clean-metadata:
	@rm $(NSTEMPLATES_DIR)/metadata.yaml 2>/dev/null || true

define git_commit
	echo "processing $(1)"
	echo "$(patsubst $(NSTEMPLATES_DIR)/%.yaml,%,$(1)):" `git log -1 --format=%h $(1)` >> $(2)
endef
