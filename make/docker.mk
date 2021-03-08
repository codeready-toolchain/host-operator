QUAY_NAMESPACE ?= ${GO_PACKAGE_ORG_NAME}
TARGET_REGISTRY := quay.io
IMAGE_TAG ?= ${GIT_COMMIT_ID_SHORT}
IMAGE ?= ${TARGET_REGISTRY}/${QUAY_NAMESPACE}/${GO_PACKAGE_REPO_NAME}:${IMAGE_TAG}
QUAY_USERNAME ?= ${QUAY_NAMESPACE}

.PHONY: docker-image
## Build the binary image
docker-image: build
	$(Q)docker build -f build/Dockerfile -t ${IMAGE} .

.PHONY: docker-push
## Push the docker image to quay.io registry
docker-push: check-namespace docker-image
	$(Q)docker push ${IMAGE}

.PHONY: podman-image
## Build the binary image
podman-image: build
	$(Q)podman build -f build/Dockerfile -t ${IMAGE} .

.PHONY: podman-push
## Push the binary image to quay.io registry
podman-push: check-namespace podman-image
	$(Q)podman push ${IMAGE}

.PHONY: check-namespace
check-namespace:
ifeq ($(QUAY_NAMESPACE),${GO_PACKAGE_ORG_NAME})
	@echo "#################################################### WARNING ####################################################"
	@echo you are going to push to $(QUAY_NAMESPACE) namespace, make sure you have set QUAY_NAMESPACE variable appropriately
	@echo "#################################################################################################################"
endif

.PHONY: docker-push-to-local
## Push the docker image to the local docker.io registry
docker-push-to-local: set-local-registry docker-image docker-push

.PHONY: set-local-registry
## Sets TARGET_REGISTRY:=docker.io
set-local-registry:
	$(eval TARGET_REGISTRY:=docker.io)

.PHONY: docker-push-to-os
## Push the docker image to the OS internal registry
docker-push-to-os: set-os-registry docker-image docker-push

.PHONY: set-os-registry
## Sets TARGET_REGISTRY:=$(shell oc get images.config.openshift.io/cluster  -o jsonpath={.status.externalRegistryHostnames[0]})
set-os-registry:
	$(eval TARGET_REGISTRY:=$(shell oc get images.config.openshift.io/cluster  -o jsonpath={.status.externalRegistryHostnames[0]}))

.PHONY: docker-login
docker-login:
	@echo "${DOCKER_PASSWORD}" | docker login quay.io -u "${QUAY_USERNAME}" --password-stdin