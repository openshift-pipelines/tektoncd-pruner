#!/usr/bin/env bash

source ./hack/version.sh

export KO_DOCKER_REPO=${KO_DOCKER_REPO:-ghcr.io/openshift-pipelines/tektoncd-pruner}
export KO_PUSH=${KO_PUSH:-false}
export LDFLAGS="${LD_FLAGS}"
export BUILDS_DIR="builds"

mkdir -p ${BUILDS_DIR}
# clears build directory
rm  -rf ${BUILDS_DIR}/*

# supported platforms
export PLATFORMS="linux/amd64,linux/s390x,linux/ppc64le,linux/arm64"

# build and resolve the image details on manifests
kustomize build config >  ${BUILDS_DIR}/release.txt
ko resolve \
  --push=${KO_PUSH} \
  --platform=${PLATFORMS} \
  --filename=${BUILDS_DIR}/release.txt \
  --tags="v${VERSION}" \
  --base-import-paths \
  --sbom=none \
  > ${BUILDS_DIR}/release-v${VERSION}.yaml

# replace version tags in the manifests
sed -i "s|pruner.tekton.dev/release: \"devel\"|pruner.tekton.dev/release: \"v${VERSION}\"|g" ${BUILDS_DIR}/release-v${VERSION}.yaml
