
PATH_TO_PUSH_NIGHTLY_FILE=scripts/push-to-quay-nightly.sh

.PHONY: push-to-quay-nightly
## Creates a new version of CSV and pushes it to quay
push-to-quay-nightly:
	$(eval PUSH_PARAMS = -pr ../host-operator/ -er https://github.com/codeready-toolchain/registration-service/ -qn ${QUAY_NAMESPACE})
ifneq ("$(wildcard ../api/$(PATH_TO_PUSH_NIGHTLY_FILE))","")
	@echo "creating release manifest in ./manifest/ directory using script from local api repo..."
	../api/${PATH_TO_PUSH_NIGHTLY_FILE} ${PUSH_PARAMS}
else
	@echo "creating release manifest in ./manifest/ directory using script from GH api repo (using latest version in master)..."
	curl -sSL https://raw.githubusercontent.com/codeready-toolchain/api/master/${PATH_TO_PUSH_NIGHTLY_FILE} | bash -s -- ${PUSH_PARAMS}
endif
