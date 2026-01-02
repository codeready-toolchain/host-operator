QUAY_NAMESPACE ?= ${GO_PACKAGE_ORG_NAME}
TARGET_REGISTRY := quay.io
IMAGE_TAG ?= ${GIT_COMMIT_ID_SHORT}
IMAGE ?= ${TARGET_REGISTRY}/${QUAY_NAMESPACE}/${GO_PACKAGE_REPO_NAME}:${IMAGE_TAG}
QUAY_USERNAME ?= ${QUAY_NAMESPACE}
IMAGE_PLATFORM ?= linux/amd64

.PHONY: podman-image
## Build the binary image
podman-image: build
	$(Q)podman build --platform ${IMAGE_PLATFORM} -f build/Dockerfile -t ${IMAGE} .

## Build the operator's image with Delve on it so that it is ready to attach a
## debugger to it.
podman-image-debug: build-debug
	$(Q) podman build --platform ${IMAGE_PLATFORM} --build-arg GOLANG_VERSION="$$(go version | awk '{print $$3}')" --file build/Dockerfile.debug --tag ${IMAGE} .

.PHONY: podman-push
## Push the binary image to quay.io registry
podman-push: check-namespace podman-image
	$(Q)podman push ${IMAGE}

.PHONY: podman-push-debug
## Push the image with the debugger in it to the repository.
podman-push-debug: check-namespace podman-image-debug
	$(Q)podman push ${IMAGE}

.PHONY: check-namespace
check-namespace:
ifeq ($(QUAY_NAMESPACE),${GO_PACKAGE_ORG_NAME})
	@echo "#################################################### WARNING ####################################################"
	@echo you are going to push to $(QUAY_NAMESPACE) namespace, make sure you have set QUAY_NAMESPACE variable appropriately
	@echo "#################################################################################################################"
endif
