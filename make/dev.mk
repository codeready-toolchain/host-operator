ifndef DEV_MK
DEV_MK:=# Prevent repeated "-include".

include ./make/verbose.mk
include ./make/git.mk

DOCKER_REPO?=quay.io/openshiftio
IMAGE_NAME?=host-operator
REGISTRY_URI=quay.io

HOST_OPERATOR_IMAGE?=quay.io/codeready-toolchain/host-operator
TIMESTAMP:=$(shell date +%s)
TAG?=$(GIT_COMMIT_ID_SHORT)-$(TIMESTAMP)
OPENSHIFT_VERSION?=4

# to watch all namespaces, keep namespace empty
APP_NAMESPACE ?= ""
LOCAL_TEST_NAMESPACE ?= "local-test"

.PHONY: local
## Deploys the operator and its CRDs on the local cluster
local: deploy-rbac build deploy-crd
	$(Q)-oc new-project $(LOCAL_TEST_NAMESPACE)
	$(Q)operator-sdk up local --namespace=$(APP_NAMESPACE)

.PHONY: local-rbac
## Setup service account and deploy RBAC on the local cluster
deploy-rbac:
	$(Q)-oc login -u system:admin
	$(Q)-oc create --namespace=$(LOCAL_TEST_NAMESPACE) -f deploy/service_account.yaml
	$(Q)-oc create --namespace=$(LOCAL_TEST_NAMESPACE) -f deploy/role.yaml
	$(Q)-oc create --namespace=$(LOCAL_TEST_NAMESPACE) -f deploy/role_binding.yaml

.PHONY: deploy-crd
## Deploy the CRDs on the local cluster
deploy-crd:
	# $(Q)-oc apply --namespace=$(LOCAL_TEST_NAMESPACE) -f deploy/crds/devconsole_v1alpha1_component_crd.yaml

.PHONY: deploy-operator
## Deploy the Operator on the local cluster
deploy-operator: 
	$(Q)oc create --namespace=$(LOCAL_TEST_NAMESPACE) -f deploy/operator.yaml

.PHONY: deploy-clean
## Deletes the local test project
deploy-clean:
	$(Q)-oc delete project $(LOCAL_TEST_NAMESPACE)

.PHONY: deploy-test
## Deploy a CR as test on the local cluster
deploy-test:
	$(Q)-oc new-project $(LOCAL_TEST_NAMESPACE)
	# $(Q)-oc apply --namespace=$(LOCAL_TEST_NAMESPACE) -f examples/devconsole_v1alpha1_component_cr.yaml

.PHONY: build-operator-image
## Build and create the operator container image
build-operator-image: ./vendor
	operator-sdk build $(HOST_OPERATOR_IMAGE):$(TAG)

.PHONY: push-operator-image
## Push the operator container image to a container registry [TODO: push to container registry on Minishift instead?]
push-operator-image: build-operator-image
	@docker login -u $(QUAY_USERNAME) -p $(QUAY_PASSWORD) $(REGISTRY_URI)
	docker push $(HOST_OPERATOR_IMAGE):$(TAG)

.PHONY: deploy-operator-only
deploy-operator-only:
	@echo "Creating Deployment for Operator"
	@cat minishift/operator.yaml | sed s/\:dev/:$(TAG)/ | oc create -f -

.PHONY: clean-all
clean-all: clean-operator clean-resources

.PHONY: clean-operator
clean-operator:
	@echo "Deleting Deployment for Operator"
	@cat minishift/operator.yaml | sed s/\:dev/:$(TAG)/ | oc delete -f - || true

.PHONY: clean-resources
clean-resources:
	@echo "Deleting sub resources..."
	@echo "Deleting ClusterRoleBinding"
	@oc delete -f ./deploy/role_binding.yaml || true
	@echo "Deleting ClusterRole"
	@oc delete -f ./deploy/role.yaml || true
	@echo "Deleting Service Account"
	@oc delete -f ./deploy/service_account.yaml || true
	@echo "Deleting Custom Resource Definitions..."
	@oc delete -f ./deploy/crds/devconsole_v1alpha1_gitsource_crd.yaml || true

.PHONY: deploy-operator
deploy-operator: build build-operator-image deploy-operator-only

.PHONY: minishift-start
minishift-start:
	minishift start --cpus 4 --memory 8GB
	-eval `minishift docker-env` && oc login -u system:admin

.PHONY: deploy-all
deploy-all: clean-resources deploy-operator

endif
