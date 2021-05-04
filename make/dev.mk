DOCKER_REPO?=quay.io/codeready-toolchain
IMAGE_NAME?=host-operator

TIMESTAMP:=$(shell date +%s)
TAG?=$(GIT_COMMIT_ID_SHORT)-$(TIMESTAMP)

# to watch all namespaces, keep namespace empty
APP_NAMESPACE ?= $(LOCAL_TEST_NAMESPACE)
LOCAL_TEST_NAMESPACE ?= "toolchain-host-operator"
ADD_CLUSTER_SCRIPT_PATH?=../toolchain-common/scripts/add-cluster.sh

.PHONY: create-namespace
## Create the test namespace
create-namespace:
	$(Q)-echo "Creating Namespace"
	$(Q)-oc new-project $(LOCAL_TEST_NAMESPACE)

.PHONY: add-member-to-host
## Run script add-cluster.sh member member-cluster
add-member-to-host:
	@${ADD_CLUSTER_SCRIPT_PATH} member member-cluster

.PHONY: add-host-to-member
## Run script add-cluster.sh host host-cluster
add-host-to-member:
	@${ADD_CLUSTER_SCRIPT_PATH} host host-cluster

.PHONY: deploy-csv
## Creates ServiceCatalog with a ConfigMap that contains operator CSV and all CRDs and image location set to quay.io
deploy-csv: docker-push
	sed -e 's|REPLACE_IMAGE|${IMAGE}|g' hack/deploy_csv.yaml | oc apply -f -

.PHONY: install-operator
## Creates OperatorGroup and Subscription that installs host operator in a test namespace
install-operator: deploy-csv create-namespace
	sed -e 's|REPLACE_NAMESPACE|${LOCAL_TEST_NAMESPACE}|g' hack/install_operator.yaml | oc apply -f -

.PHONY: deploy-csv-using-os-registry
## Creates ServiceCatalog with a ConfigMap that contains operator CSV and all CRDs and image location set to current OS registry
deploy-csv-using-os-registry: set-os-registry deploy-csv

.PHONY: install-operator-using-os-registry
## Creates OperatorGroup and Subscription that installs host operator in a test namespace
install-operator-using-os-registry: set-os-registry install-operator