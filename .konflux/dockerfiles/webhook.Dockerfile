ARG GO_BUILDER=brew.registry.redhat.io/rh-osbs/openshift-golang-builder:v1.22
ARG RUNTIME=registry.access.redhat.com/ubi9/ubi-minimal@sha256:d85040b6e3ed3628a89683f51a38c709185efc3fb552db2ad1b9180f2a6c38be

FROM $GO_BUILDER AS builder

WORKDIR /go/src/github.com/openshift-pipelines/tektoncd-pruner
COPY . .

RUN go build -v -o /tmp/webhook  ./cmd/webhook

FROM $RUNTIME
ARG VERSION=tektoncd-pruner

COPY --from=builder /tmp/webhook /ko-app/webhook


LABEL \
      com.redhat.component="openshift-pipelines-tektoncd-pruner-webhook" \
      name="openshift-pipelines/pipelines-tektoncd-pruner-rhel9" \
      version=$VERSION \
      summary="Red Hat OpenShift Pipelines Tekton Pruner Webhook" \
      maintainer="pipelines-extcomm@redhat.com" \
      description="Red Hat OpenShift Pipelines Tekton Pruner Webhook" \
      io.k8s.display-name="Red Hat OpenShift Pipelines Tekton Pruner Webhook" \
      io.k8s.description="Red Hat OpenShift Pipelines Tekton Pruner Webhook" \
      io.openshift.tags="pipelines,tekton,openshift,tektoncd-pruner-webhook"

RUN microdnf install -y shadow-utils
RUN groupadd -r -g 65532 nonroot && useradd --no-log-init -r -u 65532 -g nonroot nonroot
USER 65532
