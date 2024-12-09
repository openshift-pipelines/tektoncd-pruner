ARG GO_BUILDER=brew.registry.redhat.io/rh-osbs/openshift-golang-builder:v1.22
ARG RUNTIME=registry.access.redhat.com/ubi9/ubi-minimal@sha256:dee813b83663d420eb108983a1c94c614ff5d3fcb5159a7bd0324f0edbe7fca1

FROM $GO_BUILDER AS builder

WORKDIR /go/src/github.com/openshift-pipelines/tektoncd-pruner
COPY . .

RUN go build -v -o /tmp/controller  ./cmd/controller

FROM $RUNTIME
ARG VERSION=tektoncd-pruner

COPY --from=builder /tmp/controller /ko-app/controller


LABEL \
      com.redhat.component="openshift-pipelines-tektoncd-pruner-controller" \
      name="openshift-pipelines/pipelines-tektoncd-pruner-rhel9" \
      version=$VERSION \
      summary="Red Hat OpenShift Pipelines Tekton Pruner - Controller" \
      maintainer="pipelines-extcomm@redhat.com" \
      description="Red Hat OpenShift Pipelines Tekton Pruner - Controller" \
      io.k8s.display-name="Red Hat OpenShift Pipelines Tekton Pruner - Controller" \
      io.k8s.description="Red Hat OpenShift Pipelines Tekton Pruner - Controller" \
      io.openshift.tags="pipelines,tekton,openshift,tektoncd-pruner-controller"

RUN microdnf install -y shadow-utils
RUN groupadd -r -g 65532 nonroot && useradd --no-log-init -r -u 65532 -g nonroot nonroot
USER 65532
