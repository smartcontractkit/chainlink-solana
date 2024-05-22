ARG BASE_IMAGE
ARG IMAGE_VERSION=latest
FROM ${BASE_IMAGE}:${IMAGE_VERSION}

ARG SUITES=smoke

COPY . testdir/
WORKDIR /go/testdir
RUN /go/testdir/integration-tests/scripts/buildTests "${SUITES}"
ENTRYPOINT ["/go/testdir/integration-tests/scripts/entrypoint"]
