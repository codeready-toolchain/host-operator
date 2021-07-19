
PATH_TO_CD_GENERATE_FILE=generate-cd-release-manifests.sh
PATH_TO_BUNDLE_FILE=push-bundle-and-index-image.sh
PATH_TO_INSTALL_OPERATOR=install-operator.sh

OWNER_AND_BRANCH_LOCATION=codeready-toolchain/api/master
GH_SCRIPTS_URL=https://raw.githubusercontent.com/${OWNER_AND_BRANCH_LOCATION}/scripts

TMP_DIR?=/tmp
IMAGE_BUILDER?=podman
INDEX_IMAGE_NAME?=host-operator-index
FIRST_RELEASE=false
CHANNEL=staging
INDEX_IMAGE_TAG=latest
ENV=dev
NEXT_VERSION=0.0.1
OTHER_REPO_PATH=""
BUNDLE_TAG=""

.PHONY: push-to-quay-staging
## Creates a new version of operator bundle, adds it into an index and pushes it to quay
push-to-quay-staging: generate-cd-release-manifests push-bundle-and-index-image

.PHONY: generate-cd-release-manifests
## Generates a new version of operator manifests
generate-cd-release-manifests:
ifneq (${OTHER_REPO_PATH},"")
	$(eval OTHER_REPO_PATH_PARAM = -orp ${OTHER_REPO_PATH})
endif
	$(eval CD_GENERATE_PARAMS = -pr ../host-operator/ -er https://github.com/codeready-toolchain/registration-service -qn ${QUAY_NAMESPACE} -td ${TMP_DIR} -fr ${FIRST_RELEASE} -ch ${CHANNEL} -il ${IMAGE} -e ${ENV} ${OTHER_REPO_PATH_PARAM})
ifneq ("$(wildcard ../api/scripts/$(PATH_TO_CD_GENERATE_FILE))","")
	@echo "generating manifests for CD using script from local api repo..."
	../api/scripts/${PATH_TO_CD_GENERATE_FILE} ${CD_GENERATE_PARAMS}
else
	@echo "generating manifests for CD using script from GH api repo (using latest version in master)..."
	curl -sSL ${GH_SCRIPTS_URL}/${PATH_TO_CD_GENERATE_FILE} > /tmp/${PATH_TO_CD_GENERATE_FILE} &&	chmod +x /tmp/${PATH_TO_CD_GENERATE_FILE} && OWNER_AND_BRANCH_LOCATION=${OWNER_AND_BRANCH_LOCATION} /tmp/${PATH_TO_CD_GENERATE_FILE} ${CD_GENERATE_PARAMS}
endif

.PHONY: push-bundle-and-index-image
## Pushes generated manifests as a bundle image to quay and adds is to the image index
push-bundle-and-index-image:
ifneq (${BUNDLE_TAG},"")
	$(eval BUNDLE_TAG_PARAM = -bt ${BUNDLE_TAG})
endif
	$(eval PUSH_BUNDLE_PARAMS = -pr ../host-operator/ -er https://github.com/codeready-toolchain/registration-service -qn ${QUAY_NAMESPACE} -ch ${CHANNEL} -td ${TMP_DIR} -ib ${IMAGE_BUILDER} -iin ${INDEX_IMAGE_NAME} -iit ${INDEX_IMAGE_TAG} ${BUNDLE_TAG_PARAM})
ifneq ("$(wildcard ../api/scripts/$(PATH_TO_BUNDLE_FILE))","")
	@echo "pushing to quay in staging channel using script from local api repo..."
	../api/scripts/${PATH_TO_BUNDLE_FILE} ${PUSH_BUNDLE_PARAMS}
else
	@echo "pushing to quay in staging channel using script from GH api repo (using latest version in master)..."
	curl -sSL ${GH_SCRIPTS_URL}/${PATH_TO_BUNDLE_FILE} > /tmp/${PATH_TO_BUNDLE_FILE} && chmod +x /tmp/${PATH_TO_BUNDLE_FILE} && OWNER_AND_BRANCH_LOCATION=${OWNER_AND_BRANCH_LOCATION} /tmp/${PATH_TO_BUNDLE_FILE} ${PUSH_BUNDLE_PARAMS}
endif

.PHONY: install-operator
## Installs operator
install-operator:
	$(eval INSTALL_PARAMS = -pr ../host-operator/ -qn ${QUAY_NAMESPACE} -ch ${CHANNEL} -iin ${INDEX_IMAGE_NAME} -iit ${INDEX_IMAGE_TAG} -n ${NAMESPACE})
ifneq ("$(wildcard ../api/scripts/$(PATH_TO_INSTALL_OPERATOR))","")
	@echo "generating OLM files using script from local api repo..."
	../api/scripts/${PATH_TO_INSTALL_OPERATOR} ${INSTALL_PARAMS}
else
	@echo "generating OLM files using script from GH api repo (using latest version in master)..."
	curl -sSL ${GH_SCRIPTS_URL}/${PATH_TO_INSTALL_OPERATOR} > /tmp/${PATH_TO_INSTALL_OPERATOR} && chmod +x /tmp/${PATH_TO_INSTALL_OPERATOR} && OWNER_AND_BRANCH_LOCATION=${OWNER_AND_BRANCH_LOCATION} /tmp/${PATH_TO_INSTALL_OPERATOR} ${INSTALL_PARAMS}

endif

.PHONY: generate-rbac
generate-rbac: build controller-gen
	@echo "Re-generating ClusterRole ..."
	$(Q)$(CONTROLLER_GEN) rbac:roleName=manager-role paths=./...

.PHONY: bundle
bundle: clean-bundle generate-rbac kustomize ## Generate bundle manifests and metadata, then validate generated files.
	operator-sdk generate kustomize manifests -q
	$(KUSTOMIZE) build config/manifests | operator-sdk generate bundle --overwrite --version=${NEXT_VERSION} --channels ${CHANNEL} --default-channel ${CHANNEL} --package toolchain-host-operator
	operator-sdk bundle validate ./bundle

.PHONY: publish-current-bundle
## Pushes generated manifests as a bundle image to quay and adds is to the image index as a single release using alpha channel
publish-current-bundle: FIRST_RELEASE=true
publish-current-bundle: CHANNEL=alpha
publish-current-bundle: generate-cd-release-manifests push-bundle-and-index-image

.PHONY: install-current-operator
## Installs operator from apha channel
install-current-operator: CHANNEL=alpha
install-current-operator: install-operator