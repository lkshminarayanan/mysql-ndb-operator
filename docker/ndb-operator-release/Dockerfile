# Copyright (c) 2022, 2025, Oracle and/or its affiliates.
#
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/
ARG GOLANG_ALPINE_IMAGE=docker.io/golang:1.24.4-alpine3.22
FROM $GOLANG_ALPINE_IMAGE as builder
RUN apk add --no-cache make

WORKDIR /ndb-operator
COPY ./ /ndb-operator/
ARG gitCommit
RUN ARCH=amd64 OS=linux WITH_DEBUG=0 GIT_COMMIT_ID=$gitCommit ./hack/build.sh

FROM container-registry.oracle.com/os/oraclelinux:9-slim

COPY --from=builder /ndb-operator/bin/linux_amd64/* /usr/sbin/
COPY --from=builder /ndb-operator/vendor/github.com/go-sql-driver/* /go-sql-driver-source
COPY LICENSE .

ENTRYPOINT ["ndb-operator"]
