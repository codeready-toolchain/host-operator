# By default the project should be build under GOPATH/src/github.com/<orgname>/<reponame>
GO_PACKAGE_ORG_NAME ?= codeready-toolchain
GO_PACKAGE_REPO_NAME ?= $(shell basename $$PWD)
GO_PACKAGE_PATH ?= github.com/${GO_PACKAGE_ORG_NAME}/${GO_PACKAGE_REPO_NAME}

GO111MODULE?=on
export GO111MODULE
goarch ?= $(shell go env GOARCH)

.PHONY: format-go-code
## Formats any go file that does not match formatting defined by gofmt
format-go-code:
# The + tells find to batch multiple found files into a single gofmt invocation (like xargs),
# which is much faster than the alternative \;, which runs gofmt once per file. Removing it
# would be a syntax error — find -exec requires either + or \; as a terminator.
	$(Q)find . -name '*.go' -not -path '*/vendor/*' -not -path '*/.git/*' -exec gofmt -s -l -w {} +

.PHONY: check-go-format
## Verify the formatting defined by 'gofmt'
check-go-format:
	$(Q)find . -name '*.go' -not -path '*/vendor/*' -not -path '*/.git/*' -exec gofmt -s -l {} + 2>&1 \
		| tee $(OUT_DIR)/gofmt-errors \
		| read \
	&& echo "ERROR: These files differ from gofmt's style (run 'make format-go-code' to fix this):" \
	&& cat $(OUT_DIR)/gofmt-errors \
	&& exit 1 \
	|| true

.PHONY: build
## Build the operator
build: generate $(OUT_DIR)/operator

$(OUT_DIR)/operator:
	@echo "building host-operator in ${GO_PACKAGE_PATH}"
	$(Q)go version
	$(Q)CGO_ENABLED=0 GOARCH=${goarch} GOOS=linux \
		go build ${V_FLAG} \
		-ldflags "-X ${GO_PACKAGE_PATH}/version.Commit=${GIT_COMMIT_ID} -X ${GO_PACKAGE_PATH}/version.BuildTime=${BUILD_TIME}" \
		-o $(OUT_DIR)/bin/host-operator \
		./cmd/main.go

.PHONY: vendor
vendor:
	$(Q)go mod vendor

NSTEMPLATES_BASEDIR = deploy/templates/nstemplatetiers
NSTEMPLATES_FILES = $(wildcard $(NSTEMPLATES_BASEDIR)/**/*.yaml)

USERTEMPLATES_BASEDIR = deploy/templates/usertiers
USERTEMPLATES_TEST_BASEDIR = test/templates/usertiers

REGISTRATION_SERVICE_DIR=deploy/registration-service

.PHONY: generate
generate: generate-metadata 

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


.PHONY: verify-dependencies
## Runs commands to verify after the updated dependecies of toolchain-common/API(go mod replace), if the repo needs any changes to be made
verify-dependencies: tidy vet build test lint-go-code

.PHONY: tidy
tidy: 
	go mod tidy

.PHONY: vet
vet:
	go vet ./...

.PHONY: pre-verify
pre-verify: generate
	echo "Pre-requisite completed"
	
