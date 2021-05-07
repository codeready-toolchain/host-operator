# By default the project should be build under GOPATH/src/github.com/<orgname>/<reponame>
GO_PACKAGE_ORG_NAME ?= $(shell basename $$(dirname $$PWD))
GO_PACKAGE_REPO_NAME ?= $(shell basename $$PWD)
GO_PACKAGE_PATH ?= github.com/${GO_PACKAGE_ORG_NAME}/${GO_PACKAGE_REPO_NAME}

GO111MODULE?=on
export GO111MODULE
goarch=$(shell go env GOARCH)

.PHONY: build
## Build the operator
build: generate $(OUT_DIR)/operator

$(OUT_DIR)/operator:
	@echo "building host-operator in ${GO_PACKAGE_PATH}"
	$(Q)CGO_ENABLED=0 GOARCH=${goarch} GOOS=linux \
		go build ${V_FLAG} \
		-ldflags "-X ${GO_PACKAGE_PATH}/version.Commit=${GIT_COMMIT_ID} -X ${GO_PACKAGE_PATH}/version.BuildTime=${BUILD_TIME}" \
		-o $(OUT_DIR)/bin/host-operator \
		cmd/manager/main.go

.PHONY: vendor
vendor:
	$(Q)go mod vendor

NSTEMPLATES_BASEDIR = deploy/templates/nstemplatetiers
NSTEMPLATES_FILES = $(wildcard $(NSTEMPLATES_BASEDIR)/**/*.yaml)
NSTEMPLATES_TEST_BASEDIR = test/templates/nstemplatetiers
NSTEMPLATES_TEST_SUBDIRS = $(wildcard $(NSTEMPLATES_TEST_BASEDIR)/**/*)
NOTIFICATION_BASEDIR = deploy/templates/notificationtemplates
REGISTRATION_SERVICE_DIR=deploy/registration-service

.PHONY: generate
generate: install-go-bindata generate-metadata generate-assets 

install-go-bindata:
	@go install github.com/go-bindata/go-bindata/...

clean-metadata:
	@rm $(NSTEMPLATES_BASEDIR)/metadata.yaml 2>/dev/null || true

.PHONY: generate-metadata
generate-metadata: clean-metadata
	@echo "generating templates metadata for manifests in $(NSTEMPLATES_BASEDIR)" 
	@$(foreach tmpl,$(wildcard $(NSTEMPLATES_FILES)),$(call git_commit,$(tmpl),$(NSTEMPLATES_BASEDIR),metadata.yaml))
	
define git_commit
	echo "processing YAML files in $(2)"
	echo "$(patsubst $(2)/%.yaml,%,$(1)): \""`git log -1 --format=%h $(1)`"\"">> $(2)/$(3) # surround the commit hash with quotes to force the value as a string, even if it's a number
endef

.PHONY: generate-assets
generate-assets:
	@echo "generating bindata for files in $(NSTEMPLATES_BASEDIR) ..."
	@rm ./pkg/templates/nstemplatetiers/nstemplatetier_assets.go 2>/dev/null || true
	@$(shell go env GOPATH)/bin/go-bindata -pkg nstemplatetiers -o ./pkg/templates/nstemplatetiers/nstemplatetier_assets.go -nometadata -nocompress -prefix $(NSTEMPLATES_BASEDIR) $(NSTEMPLATES_BASEDIR)/...
	@echo "generating bindata for files in $(NSTEMPLATES_TEST_BASEDIR) ..."
	@rm ./test/templates/nstemplatetiers/nstemplatetier_assets.go 2>/dev/null || true
	@$(shell go env GOPATH)/bin/go-bindata -pkg nstemplatetiers_test -o ./test/templates/nstemplatetiers/nstemplatetier_assets.go -nometadata -nocompress -prefix $(NSTEMPLATES_TEST_BASEDIR) -ignore doc.go $(NSTEMPLATES_TEST_BASEDIR)/...
	@echo "generating notification service template data..."
	@$(shell go env GOPATH)/bin/go-bindata -pkg notificationtemplates -o ./pkg/templates/notificationtemplates/notification_assets.go -nometadata -nocompress -prefix $(NOTIFICATION_BASEDIR) -ignore doc.go $(NOTIFICATION_BASEDIR)/...
	@echo "generating registration service template data..."
	@$(shell go env GOPATH)/bin/go-bindata -pkg registrationservice -o ./pkg/controller/registrationservice/template_assets.go -nocompress -prefix $(REGISTRATION_SERVICE_DIR) $(REGISTRATION_SERVICE_DIR)

