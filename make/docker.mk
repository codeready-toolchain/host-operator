QUAY_NAMESPACE ?= ${GO_PACKAGE_ORG_NAME}
TARGET_REGISTRY := quay.io
IMAGE ?= ${TARGET_REGISTRY}/${QUAY_NAMESPACE}/${GO_PACKAGE_REPO_NAME}:${GIT_COMMIT_ID_SHORT}

.PHONY: docker-image
## Build the docker image locally that can be deployed (only contains bare operator)
docker-image: build
	$(Q)docker build -f build/Dockerfile -t ${IMAGE} .

.PHONY: docker-push
## Push the docker image to quay.io registry
docker-push: docker-image
ifeq ($(QUAY_NAMESPACE),${GO_PACKAGE_ORG_NAME})
	@echo "#################################################### WARNING ####################################################"
	@echo you are going to push to $(QUAY_NAMESPACE) namespace, make sure you have set QUAY_NAMESPACE variable appropriately
	@echo "#################################################################################################################"
endif
	$(Q)docker push ${IMAGE}

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
	@echo "${DOCKER_PASSWORD}" | docker login quay.io -u "${QUAY_NAMESPACE}" --password-stdin