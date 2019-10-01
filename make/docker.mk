IMAGE := docker.io/${GO_PACKAGE_ORG_NAME}/${GO_PACKAGE_REPO_NAME}:${GIT_COMMIT_ID_SHORT}

.PHONY: docker-image
## Build the docker image locally that can be deployed (only contains bare operator)
docker-image: build
	$(Q)docker build -f build/Dockerfile -t ${IMAGE} .

.PHONY: docker-push
## Push the docker image to the repository
docker-push: docker-image
	$(Q)docker push ${IMAGE}