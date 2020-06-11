
PATH_TO_PUSH_NIGHTLY_FILE=scripts/push-to-quay-nightly.sh
PATH_TO_OLM_GENERATE_FILE=scripts/olm-catalog-generate.sh

.PHONY: push-to-quay-nightly
## Creates a new version of CSV and pushes it to quay
push-to-quay-nightly:
	$(eval PUSH_PARAMS = -pr ../host-operator/ -er https://github.com/codeready-toolchain/registration-service/ -qn ${QUAY_NAMESPACE} -ch nightly)
ifneq ("$(wildcard ../api/$(PATH_TO_PUSH_NIGHTLY_FILE))","")
	@echo "pushing to quay in nightly channel using script from local api repo..."
	../api/${PATH_TO_PUSH_NIGHTLY_FILE} ${PUSH_PARAMS}
else
	@echo "pushing to quay in nightly channel using script from GH api repo (using latest version in master)..."
	curl -sSL https://raw.githubusercontent.com/codeready-toolchain/api/master/${PATH_TO_PUSH_NIGHTLY_FILE} | bash -s -- ${PUSH_PARAMS}
endif


.PHONY: generate-olm-files
## Regenerates base template CSV and hack files
generate-olm-files:
	$(eval GENERATE_PARAMS = -pr ../host-operator/)
ifneq ("$(wildcard ../api/$(PATH_TO_OLM_GENERATE_FILE))","")
	@echo "generating OLM files using script from local api repo..."
	../api/${PATH_TO_OLM_GENERATE_FILE} ${GENERATE_PARAMS}
else
	@echo "generating OLM files using script from GH api repo (using latest version in master)..."
	curl -sSL https://raw.githubusercontent.com/codeready-toolchain/api/master/${PATH_TO_OLM_GENERATE_FILE} | bash -s -- ${GENERATE_PARAMS}
endif