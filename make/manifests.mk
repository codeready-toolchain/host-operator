
PATH_TO_CD_GENERATE_FILE=scripts/generate-cd-release-manifests.sh
PATH_TO_BUNDLE_FILE=scripts/push-bundle-and-index-image.sh
PATH_TO_RECOVERY_FILE=scripts/recover-operator-dir.sh
PATH_TO_OLM_GENERATE_FILE=scripts/olm-catalog-generate.sh

TMP_DIR?=/tmp
IMAGE_BUILDER?=docker
INDEX_IMAGE?=hosted-toolchain-index

.PHONY: push-to-quay-staging
## Creates a new version of operator bundle, adds it into an index and pushes it to quay
push-to-quay-staging: generate-cd-release-manifests push-bundle-and-index-image recover-operator-dir

.PHONY: generate-cd-release-manifests
## Generates a new version of operator manifests
generate-cd-release-manifests:
	$(eval CD_GENERATE_PARAMS = -pr ../host-operator/ -er https://github.com/codeready-toolchain/registration-service -qn ${QUAY_NAMESPACE} -td ${TMP_DIR})
ifneq ("$(wildcard ../api/$(PATH_TO_CD_GENERATE_FILE))","")
	@echo "generating manifests for CD using script from local api repo..."
	../api/${PATH_TO_CD_GENERATE_FILE} ${CD_GENERATE_PARAMS}
else
	@echo "generating manifests for CD using script from GH api repo (using latest version in master)..."
	curl -sSL https://raw.githubusercontent.com/codeready-toolchain/api/master/${PATH_TO_CD_GENERATE_FILE} | bash -s -- ${CD_GENERATE_PARAMS}
endif

.PHONY: push-bundle-and-index-image
## Pushes generated manifests as a bundle image to quay and adds is to the image index
push-bundle-and-index-image:
	$(eval PUSH_BUNDLE_PARAMS = -pr ../host-operator/ -er https://github.com/codeready-toolchain/registration-service -qn ${QUAY_NAMESPACE} -ch staging -td ${TMP_DIR} -ib ${IMAGE_BUILDER} -im ${INDEX_IMAGE})
ifneq ("$(wildcard ../api/$(PATH_TO_BUNDLE_FILE))","")
	@echo "pushing to quay in staging channel using script from local api repo..."
	../api/${PATH_TO_BUNDLE_FILE} ${PUSH_BUNDLE_PARAMS}
else
	@echo "pushing to quay in staging channel using script from GH api repo (using latest version in master)..."
	curl -sSL https://raw.githubusercontent.com/codeready-toolchain/api/master/${PATH_TO_BUNDLE_FILE} | bash -s -- ${PUSH_BUNDLE_PARAMS}
endif

.PHONY: recover-operator-dir
## Recovers the operator directory from the backup folder
recover-operator-dir:
	$(eval RECOVERY_PARAMS = -pr ../host-operator/ -td ${TMP_DIR})
ifneq ("$(wildcard ../api/$(PATH_TO_RECOVERY_FILE))","")
	@echo "recovering the operator directory from the backup folder using script from local api repo..."
	../api/${PATH_TO_RECOVERY_FILE} ${RECOVERY_PARAMS}
else
	@echo "recovering the operator directory from the backup folder script from GH api repo (using latest version in master)..."
	curl -sSL https://raw.githubusercontent.com/codeready-toolchain/api/master/${PATH_TO_RECOVERY_FILE} | bash -s -- ${RECOVERY_PARAMS}
endif


.PHONY: generate-olm-files
## Regenerates base template CSV and hack files
generate-olm-files: generate-rbac
	$(eval GENERATE_PARAMS = -pr ../host-operator/)
ifneq ("$(wildcard ../api/$(PATH_TO_OLM_GENERATE_FILE))","")
	@echo "generating OLM files using script from local api repo..."
	../api/${PATH_TO_OLM_GENERATE_FILE} ${GENERATE_PARAMS}
else
	@echo "generating OLM files using script from GH api repo (using latest version in master)..."
	curl -sSL https://raw.githubusercontent.com/codeready-toolchain/api/master/${PATH_TO_OLM_GENERATE_FILE} | bash -s -- ${GENERATE_PARAMS}
endif

.PHONY: generate-rbac
generate-rbac: controller-gen
	@echo "Re-generating the deepcopy go file & the Toolchain CRD files... "
	$(Q)$(CONTROLLER_GEN) rbac:roleName=host-operator paths=./...
	mv config/rbac/role.yaml deploy/cluster_role.yaml