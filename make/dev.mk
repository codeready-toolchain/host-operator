ifndef DEV_MK
DEV_MK:=# Prevent repeated "-include".

include ./make/verbose.mk
include ./make/git.mk

DOCKER_REPO?=quay.io/codeready-toolchain
IMAGE_NAME?=host-operator

TIMESTAMP:=$(shell date +%s)
TAG?=$(GIT_COMMIT_ID_SHORT)-$(TIMESTAMP)

# ToDo: add required make target to build and run operator in dev environment later once we have openshift-ci in place.


endif
