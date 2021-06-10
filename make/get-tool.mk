CONTROLLER_GEN_VERSION=v0.4.1

CONTROLLER_GEN = $(shell pwd)/bin/controller-gen
controller-gen: ## Download controller-gen locally if necessary.
	$(call go-get-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,${CONTROLLER_GEN_VERSION})

# go-get-tool will 'go get' any package $2 with version $3 and install it to $1.
PROJECT_DIR := $(shell pwd)
VERSIONS_FILE := $(PROJECT_DIR)/bin/version

define go-get-tool
@if [[ ! -f ${1} ]] || [[ ! -f ${VERSIONS_FILE} ]] || [[ -z $$(grep ${2} ${VERSIONS_FILE} | grep '${3}$$') ]]; then \
	set -e ;\
	TMP_DIR=$$(mktemp -d) ;\
	cd $${TMP_DIR} ;\
	go mod init tmp ;\
	echo "Downloading ${2}" ;\
	GOBIN=$(PROJECT_DIR)/bin go get ${2}@${3} ;\
	touch ${VERSIONS_FILE} ;\
	sed '\|${2}|d' ${VERSIONS_FILE} > $${TMP_DIR}/versions ;\
	mv $${TMP_DIR}/versions ${VERSIONS_FILE} ;\
	echo "${2} ${3}" >> ${VERSIONS_FILE} ;\
	rm -rf $$TMP_DIR ;\
fi
endef
