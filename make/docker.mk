.PHONY: docker-image
## Build the docker image locally that can be deployed (only contains bare operator)
docker-image: build
	$(Q)docker build -f build/Dockerfile -t docker.io/${GO_PACKAGE_ORG_NAME}/${GO_PACKAGE_REPO_NAME}:${GIT_COMMIT_ID_SHORT} .
