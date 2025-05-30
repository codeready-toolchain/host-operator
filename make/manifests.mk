TMP_DIR?=/tmp
IMAGE_BUILDER?=podman
IMAGE_PLATFORM?=linux/amd64
INDEX_IMAGE_NAME?=host-operator-index
FIRST_RELEASE?=false
CHANNEL=staging
INDEX_IMAGE_TAG=latest
NEXT_VERSION=0.0.1
OTHER_REPO_PATH=""
BUNDLE_TAG=""
REPLACE_OPERATOR_VERSION=""

.PHONY: push-to-quay-staging
## Creates a new version of operator bundle, adds it into an index and pushes it to quay
push-to-quay-staging: generate-cd-release-manifests push-bundle-and-index-image

.PHONY: generate-cd-release-manifests
## Generates a new version of operator manifests
generate-cd-release-manifests:
ifneq (${OTHER_REPO_PATH},"")
	$(eval OTHER_REPO_PATH_PARAM = -orp ${OTHER_REPO_PATH})
endif
ifneq (${REPLACE_OPERATOR_VERSION},"")
	$(eval REPLACE_OPERATOR_VERSION_PARAM = -rv ${REPLACE_OPERATOR_VERSION})
endif
	$(MAKE) run-cicd-script SCRIPT_PATH=scripts/cd/generate-cd-release-manifests.sh SCRIPT_PARAMS="-pr ../host-operator/ -er https://github.com/codeready-toolchain/registration-service -qn ${QUAY_NAMESPACE} -td ${TMP_DIR} -fr ${FIRST_RELEASE} -ch ${CHANNEL} -il ${IMAGE} ${OTHER_REPO_PATH_PARAM} ${REPLACE_OPERATOR_VERSION_PARAM}"

.PHONY: push-bundle-and-index-image
## Pushes generated manifests as a bundle image to quay and adds is to the image index
push-bundle-and-index-image:
ifneq (${BUNDLE_TAG},"")
	$(eval BUNDLE_TAG_PARAM = -bt ${BUNDLE_TAG})
endif
	$(MAKE) run-cicd-script SCRIPT_PATH=scripts/cd/push-bundle-and-index-image.sh SCRIPT_PARAMS="-pr ../host-operator/ -er https://github.com/codeready-toolchain/registration-service -qn ${QUAY_NAMESPACE} -ch ${CHANNEL} -td ${TMP_DIR} -ib ${IMAGE_BUILDER} -iin ${INDEX_IMAGE_NAME} -iit ${INDEX_IMAGE_TAG} -ip ${IMAGE_PLATFORM} ${BUNDLE_TAG_PARAM}"

.PHONY: generate-rbac
generate-rbac: build controller-gen
	@echo "Re-generating ClusterRole ..."
	$(Q)$(CONTROLLER_GEN) rbac:roleName=manager-role paths=./...

.PHONY: bundle
bundle: clean-bundle generate-rbac kustomize ## Generate bundle manifests and metadata, then validate generated files.
	operator-sdk generate kustomize manifests -q
	$(eval TMP_MANIFEST_FILE := $(shell mktemp))
	@echo "generating manifests to temporary file: ${TMP_MANIFEST_FILE}"
	$(KUSTOMIZE) build config/manifests -o ${TMP_MANIFEST_FILE}
	cat ${TMP_MANIFEST_FILE} | operator-sdk generate bundle --overwrite --version=${NEXT_VERSION} --channels ${CHANNEL} --default-channel ${CHANNEL} --package toolchain-host-operator
	operator-sdk bundle validate ./bundle

.PHONY: publish-current-bundle
## Pushes generated manifests as a bundle image to quay and adds is to the image index as a single release using alpha channel
publish-current-bundle: FIRST_RELEASE=true
publish-current-bundle: CHANNEL=alpha
publish-current-bundle: generate-cd-release-manifests push-bundle-and-index-image
