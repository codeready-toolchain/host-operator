############################################################
#
# (local) Tests
#
############################################################

.PHONY: test
## runs the tests without coverage and excluding E2E tests
test: generate
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
test-with-coverage: generate
	@echo "running the tests with coverage..."
	@-mkdir -p $(COV_DIR)
	@-rm $(COV_DIR)/coverage.txt
	$(Q)go test -vet off ${V_FLAG} $(shell go list ./... | grep -v /cmd/manager) -coverprofile=$(COV_DIR)/coverage.txt -covermode=atomic ./...

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

E2E_REPO_PATH := ""

.PHONY: test-e2e-local
test-e2e-local: generate
	$(MAKE) test-e2e E2E_REPO_PATH=../toolchain-e2e

.PHONY: publish-current-bundles-for-e2e
publish-current-bundles-for-e2e: generate get-e2e-repo
	# build & publish the bundles via toolchain-e2e repo
	$(MAKE) -C ${E2E_REPO_PATH} get-and-publish-operators HOST_REPO_PATH=${PWD}

.PHONY: test-e2e
test-e2e: generate get-e2e-repo
	# run the e2e test via toolchain-e2e repo
	$(MAKE) -C ${E2E_REPO_PATH} test-e2e HOST_REPO_PATH=${PWD}

.PHONY: get-e2e-repo
get-e2e-repo:
ifeq ($(E2E_REPO_PATH),"")
	# set e2e repo path to tmp directory
	$(eval E2E_REPO_PATH = /tmp/toolchain-e2e)
	# delete to have clear environment
	rm -rf ${E2E_REPO_PATH}
	# clone
	git clone https://github.com/codeready-toolchain/toolchain-e2e.git ${E2E_REPO_PATH}
    ifneq ($(CI),)
        ifneq ($(GITHUB_ACTIONS),)
			$(eval BRANCH_REF = refs/heads/${GITHUB_HEAD_REF})
			$(eval AUTHOR_LINK = https://github.com/${GITHUB_ACTOR})
        else
			$(eval AUTHOR_LINK = $(shell jq -r '.refs[0].pulls[0].author_link' <<< $${CLONEREFS_OPTIONS} | tr -d '[:space:]'))
			@echo "using pull sha ${PULL_PULL_SHA}"
			# get branch ref of the fork the PR was created from
			$(eval BRANCH_REF := $(shell curl ${AUTHOR_LINK}/host-operator.git/info/refs?service=git-upload-pack --output - /dev/null 2>&1 | grep -a ${PULL_PULL_SHA} | awk '{print $$2}'))
        endif
		@echo "using author link ${AUTHOR_LINK}"
		@echo "detected branch ref ${BRANCH_REF}"
		# check if a branch with the same ref exists in the user's fork of toolchain-e2e repo
		$(eval REMOTE_E2E_BRANCH := $(shell curl ${AUTHOR_LINK}/toolchain-e2e.git/info/refs?service=git-upload-pack --output - 2>/dev/null | grep -a "${BRANCH_REF}$$" | awk '{print $$2}'))
		@echo "branch ref of the user's fork: \"${REMOTE_E2E_BRANCH}\" - if empty then not found"
		# check if the branch with the same name exists, if so then merge it with master and use the merge branch, if not then use master
		if [[ -n "${REMOTE_E2E_BRANCH}" ]]; then \
			git config --global user.email "devtools@redhat.com"; \
			git config --global user.name "Devtools"; \
			# retrieve the branch name \
			BRANCH_NAME=`echo ${BRANCH_REF} | awk -F'/' '{print $$3}'`; \
			# add the user's fork as remote repo \
			git --git-dir=${E2E_REPO_PATH}/.git --work-tree=${E2E_REPO_PATH} remote add external ${AUTHOR_LINK}/toolchain-e2e.git; \
			# fetch the branch \
			git --git-dir=${E2E_REPO_PATH}/.git --work-tree=${E2E_REPO_PATH} fetch external ${BRANCH_REF}; \
			# merge the branch with master \
			git --git-dir=${E2E_REPO_PATH}/.git --work-tree=${E2E_REPO_PATH} merge --allow-unrelated-histories --no-commit FETCH_HEAD; \
		fi;
    endif
endif

.PHONY: clean-e2e-namespaces
clean-e2e-namespaces:
	$(Q)-oc get projects --output=name | grep -E "(member|host)\-operator\-[0-9]+|toolchain\-e2e\-[0-9]+" | xargs oc delete
