# Define the default build as "prod", so that in the case that no build
# arguments are provided, we build the optimized image.
ARG BUILD_TYPE='prod'

# Default production build steps for the operator.
FROM registry.access.redhat.com/ubi8/ubi-minimal:latest AS prod-build

LABEL maintainer="KubeSaw <devsandbox@redhat.com>"
LABEL author="KubeSaw <devsandbox@redhat.com>"

ENV OPERATOR=/usr/local/bin/host-operator \
    USER_UID=1001 \
    USER_NAME=host-operator \
    LANG=en_US.utf8

# install operator binary
COPY build/_output/bin/host-operator ${OPERATOR}

COPY build/bin /usr/local/bin
RUN  /usr/local/bin/user_setup

ENTRYPOINT ["/usr/local/bin/entrypoint"]

# A throwaway intermediate path which compiles Delve with the same Golang
# version as the host computer. That ensures no discrepancies in the code
# when setting up breakpoints when debugging.
FROM registry.access.redhat.com/ubi8/ubi-minimal:latest AS debug-initial-build

# In this build path, the Golang version must be provided as a build argument,
# which will ensure that Delve gets built with the same version the operator's
# binary gets built with, to avoid any discrepancies.
ARG GOLANG_VERSION
RUN test -n "${GOLANG_VERSION}" || (echo 'The GOLANG_VERSION build parameter is required to build the debug image. Please provide it with "--build-arg GOLANG_VERSION=goX.Y.Z"' && exit 1)

# Install the tools to be able to download and extract Golang.
RUN microdnf install --assumeyes \
    curl \
    gzip \
    tar \
    && microdnf clean all

# The target architecture in which the binary will be built.
ARG TARGET_ARCH='linux-amd64'

# Download and extract Golang.
RUN curl --location --output "/tmp/${GOLANG_VERSION}.${TARGET_ARCH}.tar.gz" "https://go.dev/dl/${GOLANG_VERSION}.${TARGET_ARCH}.tar.gz" \
    && tar --directory /usr/local --extract --file "/tmp/${GOLANG_VERSION}.${TARGET_ARCH}.tar.gz" \
    && rm --force "/tmp/${GOLANG_VERSION}.${TARGET_ARCH}.tar.gz"

# Put Golang in the path so that we can compile Delve with it.
ENV PATH=$PATH:/usr/local/go/bin

# Build Delve and leave it in a temporary directory.
RUN GOBIN=/tmp/bin go install github.com/go-delve/delve/cmd/dlv@latest

# In the case that the "BUILD_TYPE=debug" build argument is provided, we
# "attempt to copy" the Delve executable into the production image. Since that
# creates a dependency with the "debug-initial-build", that path is executed
# by building the Delve executable.
FROM prod-build AS debug-build
COPY --from=debug-initial-build /tmp/bin/dlv /usr/local/bin/dlv

# When the no build argument is provided, we simply fall back to the
# production build which comes with the optimized and minimized image.
FROM ${BUILD_TYPE}-build AS final-build

USER ${USER_UID}
