
.PHONY: deploy-host
## Deploy Operator on dev cluster
deploy-host: create-namespace deploy-rbac deploy-crd
	oc apply -f deploy/operator_dev_cluster.yaml
