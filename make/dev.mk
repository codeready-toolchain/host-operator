DOCKER_REPO?=quay.io/codeready-toolchain
IMAGE_NAME?=host-operator

TIMESTAMP:=$(shell date +%s)
TAG?=$(GIT_COMMIT_ID_SHORT)-$(TIMESTAMP)

# to watch all namespaces, keep namespace empty
APP_NAMESPACE ?= $(LOCAL_TEST_NAMESPACE)
LOCAL_TEST_NAMESPACE ?= "toolchain-host-operator"
ADD_CLUSTER_SCRIPT_PATH?=../toolchain-common/scripts/add-cluster.sh

.PHONY: up-local
## Run Operator locally
up-local: login-as-admin create-namespace deploy-rbac build deploy-crd
	$(Q)-oc new-project $(LOCAL_TEST_NAMESPACE) || true
	$(Q)operator-sdk up local --namespace=$(APP_NAMESPACE) --verbose

.PHONY: login-as-admin
## Log in as system:admin
login-as-admin:
	$(Q)-echo "Logging using system:admin..."
	$(Q)-oc login -u system:admin

.PHONY: create-namespace
## Create the test namespace
create-namespace:
	$(Q)-echo "Creating Namespace"
	$(Q)-oc new-project $(LOCAL_TEST_NAMESPACE)

.PHONY: use-namespace
## Log in as system:admin and enter the test namespace
use-namespace: login-as-admin
	$(Q)-echo "Using to the namespace $(LOCAL_TEST_NAMESPACE)"
	$(Q)-oc project $(LOCAL_TEST_NAMESPACE)

.PHONY: clean-namespace
## Delete the test namespace
clean-namespace:
	$(Q)-echo "Deleting Namespace"
	$(Q)-oc delete project $(LOCAL_TEST_NAMESPACE)

.PHONY: reset-namespace
## Delete an create the test namespace and deploy rbac there
reset-namespace: login-as-admin clean-namespace create-namespace deploy-rbac

.PHONY: deploy-rbac
## Setup service account and deploy RBAC
deploy-rbac:
	$(Q)-oc apply -f deploy/service_account.yaml
	$(Q)-oc apply -f deploy/role.yaml
	$(Q)-oc apply -f deploy/role_binding.yaml
	$(Q)-oc apply -f deploy/cluster_role.yaml
	$(Q)-sed -e 's|REPLACE_NAMESPACE|${LOCAL_TEST_NAMESPACE}|g' ./deploy/cluster_role_binding.yaml  | oc apply -f -

.PHONY: deploy-crd
## Deploy CRD
deploy-crd:
	$(Q)-oc apply -f deploy/crds

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