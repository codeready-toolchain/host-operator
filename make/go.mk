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
	@echo "building host-operator in ${GO_PACKAGE_PATH}"
	$(Q)CGO_ENABLED=0 GOARCH=amd64 GOOS=linux \
		go build ${V_FLAG} \
		-ldflags "-X ${GO_PACKAGE_PATH}/version.Commit=${GIT_COMMIT_ID} -X ${GO_PACKAGE_PATH}/version.BuildTime=${BUILD_TIME}" \
		-o $(OUT_DIR)/bin/host-operator \
		cmd/manager/main.go

.PHONY: vendor
vendor:
	$(Q)go mod vendor

NSTEMPLATES_DIR=deploy/templates/nstemplatetiers
NSTEMPLATES_NAMESPACES_DIR=$(NSTEMPLATES_DIR)/namespaces
NSTEMPLATES_CLUSTERRESOURCEQUOTAS_DIR=$(NSTEMPLATES_DIR)/clusterresources
NSTEMPLATES_TEST_DIR=test/templates/nstemplatetiers
NSTEMPLATES_NAMESPACES_TEST_DIR=$(NSTEMPLATES_TEST_DIR)/namespaces
NSTEMPLATES_CLUSTERRESOURCEQUOTAS_TEST_DIR=$(NSTEMPLATES_TEST_DIR)/clusterresources
REGISTRATION_SERVICE_DIR=deploy/registration-service

.PHONY: generate
generate: generate-metadata generate-assets 

.PHONY: generate-assets
generate-assets:
	@echo "generating templates bindata..."
	@go install github.com/go-bindata/go-bindata/...
	@$(GOPATH)/bin/go-bindata -pkg namespaces -o ./pkg/templates/nstemplatetiers/namespaces/namespaces_assets.go -nocompress -prefix $(NSTEMPLATES_NAMESPACES_DIR) $(NSTEMPLATES_NAMESPACES_DIR)
	@$(GOPATH)/bin/go-bindata -pkg clusterresources -o ./pkg/templates/nstemplatetiers/clusterresources/clusterresources_assets.go -nocompress -prefix $(NSTEMPLATES_CLUSTERRESOURCEQUOTAS_DIR) $(NSTEMPLATES_CLUSTERRESOURCEQUOTAS_DIR)
	@echo "generating test templates bindata..."
	@$(GOPATH)/bin/go-bindata -pkg nstemplatetiers_test -o ./test/templates/nstemplatetiers/namespaces/namespaces_assets.go -nocompress -prefix $(NSTEMPLATES_NAMESPACES_TEST_DIR) $(NSTEMPLATES_NAMESPACES_TEST_DIR)
	@$(GOPATH)/bin/go-bindata -pkg nstemplatetiers_test -o ./test/templates/nstemplatetiers/clusterresources/clusterresources_assets.go -nocompress -prefix $(NSTEMPLATES_CLUSTERRESOURCEQUOTAS_TEST_DIR) $(NSTEMPLATES_CLUSTERRESOURCEQUOTAS_TEST_DIR)
	@echo "generating registration service template data..."
	@$(GOPATH)/bin/go-bindata -pkg registrationservice -o ./pkg/controller/registrationservice/template_assets.go -nocompress -prefix $(REGISTRATION_SERVICE_DIR) $(REGISTRATION_SERVICE_DIR)

.PHONY: generate-metadata
generate-metadata: clean-metadata
	@echo "generating namespace templates metadata for manifests in $(NSTEMPLATES_NAMESPACES_DIR)" 
	@$(foreach tmpl,$(wildcard $(NSTEMPLATES_NAMESPACES_DIR)/*.yaml),$(call git_commit,$(tmpl),$(NSTEMPLATES_NAMESPACES_DIR),metadata.yaml);)
	@$(foreach tmpl,$(wildcard $(NSTEMPLATES_CLUSTERRESOURCEQUOTAS_DIR)/*.yaml),$(call git_commit,$(tmpl),$(NSTEMPLATES_CLUSTERRESOURCEQUOTAS_DIR),metadata.yaml);)

clean-metadata:
	@rm $(NSTEMPLATES_NAMESPACES_DIR)/metadata.yaml 2>/dev/null || true
	@rm $(NSTEMPLATES_CLUSTERRESOURCEQUOTAS_DIR)/metadata.yaml 2>/dev/null || true

define git_commit
	echo "processing YAML files in $(2)"
	echo "$(patsubst $(2)/%.yaml,%,$(1)): \""`git log -1 --format=%h $(1)`"\"">> $(2)/$(3) # surround the commit hash with quotes to force the value as a string, even if it's a number
endef
