OWNER_AND_BRANCH_LOCATION=rsoaresd/toolchain-cicd/update_4_20_cicd
GH_SCRIPTS_URL=https://raw.githubusercontent.com/${OWNER_AND_BRANCH_LOCATION}

.PHONY: run-cicd-script
## Runs script from toolchain-cicd repo. If the script is found locally, then it runs the local version. If not, then it downloads the script from master
run-cicd-script:
ifneq ("$(wildcard ../toolchain-cicd/${SCRIPT_PATH})","")
	@echo "running the script from local toolchain-cicd repo..."
	../toolchain-cicd/${SCRIPT_PATH} ${SCRIPT_PARAMS}
else
	@echo "running the script from GH api repo (using latest version in master)..."
	$(eval SCRIPT_NAME := $(shell basename ${SCRIPT_PATH}))
	curl -sSL ${GH_SCRIPTS_URL}/${SCRIPT_PATH} > /tmp/${SCRIPT_NAME} && chmod +x /tmp/${SCRIPT_NAME} && OWNER_AND_BRANCH_LOCATION=${OWNER_AND_BRANCH_LOCATION} /tmp/${SCRIPT_NAME} ${SCRIPT_PARAMS}
endif