.PHONY: verify-dependencies
## Runs commands to verify after the updated dependecies of toolchain-common/API(go mod replace), if the repo needs any changes to be made
verify-dependencies: tidy generate vet build test

.PHONY: tidy
tidy: 
	go mod tidy

.PHONY: vet
vet:
	go vet ./...
	