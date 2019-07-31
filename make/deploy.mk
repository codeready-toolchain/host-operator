
.PHONY: deploy-host
## Run Operator locally
deploy-host: create-namespace deploy-rbac deploy-crd
	oc apply -f deploy/operator_dev_cluster.yaml
