ARG BASE_IMAGE
ARG IMAGE_VERSION=latest
FROM ${BASE_IMAGE}:${IMAGE_VERSION}

ARG SUITES=smoke soak

COPY . testdir/
WORKDIR /go/testdir
RUN /go/testdir/tests/scripts/buildTests "${SUITES}"
ENTRYPOINT ["/go/testdir/tests/scripts/entrypoint"]
