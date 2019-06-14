ifndef TEST_MK
TEST_MK:=# Prevent repeated "-include".
UNAME_S := $(shell uname -s)

include ./make/verbose.mk
include ./make/out.mk

.PHONY: test
## Runs Go package tests and stops when the first one fails
test:
	$(Q)go test -vet off ${V_FLAG} $(shell go list ./... | grep -v /test/e2e) -failfast

.PHONY: test-coverage
## Runs Go package tests and produces coverage information
test-coverage: ./out/cover.out

.PHONY: test-coverage-html
## Gather (if needed) coverage information and show it in your browser
test-coverage-html: ./out/cover.out
	$(Q)go tool cover -html=./out/cover.out

./out/cover.out:
	$(Q)go test ${V_FLAG} -race $(shell go list ./... | grep -v /test/e2e) -failfast -coverprofile=cover.out -covermode=atomic -outputdir=./out


endif
