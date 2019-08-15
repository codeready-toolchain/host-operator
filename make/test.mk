############################################################
#
# (local) Tests
#
############################################################

.PHONY: test
## runs the tests without coverage and excluding E2E tests
test:
	@echo "running the tests without coverage and excluding E2E tests..."
	$(Q)go test ${V_FLAG} -race $(shell go list ./... | grep -v /test/e2e) -failfast


############################################################
#
# OpenShift CI Tests with Coverage
#
############################################################

# Output directory for coverage information
COV_DIR = $(OUT_DIR)/coverage

.PHONY: test-with-coverage
## runs the tests with coverage
test-with-coverage:
	@echo "running the tests with coverage..."
	@-mkdir -p $(COV_DIR)
	@-rm $(COV_DIR)/coverage.txt
	$(Q)go test -vet off ${V_FLAG} $(shell go list ./... | grep -v /test/e2e) -coverprofile=$(COV_DIR)/coverage.txt -covermode=atomic ./...

.PHONY: upload-codecov-report
# Uploads the test coverage reports to codecov.io. 
# DO NOT USE LOCALLY: must only be called by OpenShift CI when processing new PR and when a PR is merged! 
upload-codecov-report: 
	# Upload coverage to codecov.io. Since we don't run on a supported CI platform (Jenkins, Travis-ci, etc.), 
	# we need to provide the PR metadata explicitely using env vars used coming from https://github.com/openshift/test-infra/blob/master/prow/jobs.md#job-environment-variables
	# 
	# Also: not using the `-F unittests` flag for now as it's temporarily disabled in the codecov UI 
	# (see https://docs.codecov.io/docs/flags#section-flags-in-the-codecov-ui)
	env
ifneq ($(PR_COMMIT), null)
	@echo "uploading test coverage report for pull-request #$(PULL_NUMBER)..."
	bash <(curl -s https://codecov.io/bash) \
		-t $(CODECOV_TOKEN) \
		-f $(COV_DIR)/coverage.txt \
		-C $(PR_COMMIT) \
		-r $(REPO_OWNER)/$(REPO_NAME) \
		-P $(PULL_NUMBER) \
		-Z
else
	@echo "uploading test coverage report after PR was merged..."
	bash <(curl -s https://codecov.io/bash) \
		-t $(CODECOV_TOKEN) \
		-f $(COV_DIR)/coverage.txt \
		-C $(BASE_COMMIT) \
		-r $(REPO_OWNER)/$(REPO_NAME) \
		-Z
endif

CODECOV_TOKEN := "e0747034-8ed2-4165-8d0b-3015d94307f9"
REPO_OWNER := $(shell echo $$CLONEREFS_OPTIONS | jq '.refs[0].org')
REPO_NAME := $(shell echo $$CLONEREFS_OPTIONS | jq '.refs[0].repo')
BASE_COMMIT := $(shell echo $$CLONEREFS_OPTIONS | jq '.refs[0].base_sha')
PR_COMMIT := $(shell echo $$CLONEREFS_OPTIONS | jq '.refs[0].pulls[0].sha')
PULL_NUMBER := $(shell echo $$CLONEREFS_OPTIONS | jq '.refs[0].pulls[0].number')

###########################################################
#
# End-to-end Tests
#
###########################################################

.PHONY: test-e2e
test-e2e:  deploy-member e2e-setup setup-kubefed
	sed -e 's|REPLACE_IMAGE|${IMAGE_NAME}|g' ./deploy/operator.yaml  | oc apply -f -
	MEMBER_NS=${MEMBER_NS} operator-sdk test local ./test/e2e --no-setup --namespace $(TEST_NAMESPACE) --verbose --go-test-flags "-timeout=15m"
	oc get kubefedcluster -n $(TEST_NAMESPACE)
	oc get kubefedcluster -n $(MEMBER_NS)

.PHONY: e2e-setup
e2e-setup: get-test-namespace is-minishift
	oc new-project $(TEST_NAMESPACE) --display-name e2e-tests
	oc apply -f ./deploy/service_account.yaml
	oc apply -f ./deploy/role.yaml
	cat ./deploy/role_binding.yaml | sed s/\REPLACE_NAMESPACE/$(TEST_NAMESPACE)/ | oc apply -f -
	oc apply -f deploy/crds

.PHONY: is-minishift
is-minishift:
ifeq ($(OPENSHIFT_BUILD_NAMESPACE),)
	$(info logging as system:admin")
	$(shell echo "oc login -u system:admin")
	$(eval IMAGE_NAME := docker.io/${GO_PACKAGE_ORG_NAME}/${GO_PACKAGE_REPO_NAME}:${GIT_COMMIT_ID_SHORT})
	$(shell echo "make docker-image")
else
	$(eval IMAGE_NAME := registry.svc.ci.openshift.org/${OPENSHIFT_BUILD_NAMESPACE}/stable:host-operator)
endif

.PHONY: setup-kubefed
setup-kubefed:
	curl -sSL https://raw.githubusercontent.com/codeready-toolchain/toolchain-common/master/scripts/add-cluster.sh | bash -s -- -t member -mn $(MEMBER_NS) -hn $(TEST_NAMESPACE) -s
	curl -sSL https://raw.githubusercontent.com/codeready-toolchain/toolchain-common/master/scripts/add-cluster.sh | bash -s -- -t host -mn $(MEMBER_NS) -hn $(TEST_NAMESPACE) -s

.PHONY: e2e-cleanup
e2e-cleanup:
	$(eval TEST_NAMESPACE := $(shell cat $(OUT_DIR)/test-namespace))
	$(Q)-oc delete project $(TEST_NAMESPACE) --timeout=10s --wait

.PHONY: get-test-namespace
get-test-namespace: $(OUT_DIR)/test-namespace
	$(eval TEST_NAMESPACE := $(shell cat $(OUT_DIR)/test-namespace))

$(OUT_DIR)/test-namespace:
	@echo -n "host-operator-$(shell date +'%s')" > $(OUT_DIR)/test-namespace

###########################################################
#
# Deploying Member Operator in Openshift CI Environment
#
###########################################################

.PHONY: deploy-member
deploy-member:
	$(eval MEMBER_NS := $(shell echo -n "member-operator-$(shell date +'%s')"))
	rm -rf /tmp/member-operator
	# cloning here as don't want to maintain it for every single change in deploy directory of member-operator
	git clone https://github.com/codeready-toolchain/member-operator.git --depth 1 /tmp/member-operator
	oc new-project $(MEMBER_NS)
	oc apply -f /tmp/member-operator/deploy/service_account.yaml
	oc apply -f /tmp/member-operator/deploy/role.yaml
	cat /tmp/member-operator/deploy/role_binding.yaml | sed s/\REPLACE_NAMESPACE/$(MEMBER_NS)/ | oc apply -f -
	oc apply -f /tmp/member-operator/deploy/crds
	sed -e 's|REPLACE_IMAGE|registry.svc.ci.openshift.org/codeready-toolchain/member-operator-v0.1:member-operator|g' /tmp/member-operator/deploy/operator.yaml  | oc apply -f -
