ARG GO_BUILDER=brew.registry.redhat.io/rh-osbs/openshift-golang-builder:v1.24
ARG RUNTIME=registry.access.redhat.com/ubi9/ubi-minimal:latest@sha256:90bd85dcd061d1ad6dbda70a867c41958c04a86462d05c631f8205e8870f28f8

FROM $GO_BUILDER AS builder

WORKDIR /go/src/github.com/openshift-pipelines/tektoncd-pruner
COPY upstream .

ENV GODEBUG="http2server=0"
RUN go build -ldflags="-X 'knative.dev/pkg/changeset.rev=$(cat HEAD)'" -mod=vendor -tags disable_gcp -v -o /tmp/webhook \
    ./cmd/webhook

FROM $RUNTIME
ARG VERSION=tektoncd-pruner-next

ENV KO_APP=/ko-app \
    WEBHOOK=${KO_APP}/webhook

COPY --from=builder /tmp/webhook ${WEBHOOK}

LABEL \
      com.redhat.component="openshift-pipelines-pruner-webhook-rhel9-container" \
      cpe="cpe:/a:redhat:openshift_pipelines:1.18::el9" \
      description="Red Hat OpenShift Pipelines tektoncd-pruner webhook" \
      io.k8s.description="Red Hat OpenShift Pipelines tektoncd-pruner webhook" \
      io.k8s.display-name="Red Hat OpenShift Pipelines tektoncd-pruner webhook" \
      io.openshift.tags="tekton,openshift,tektoncd-pruner,webhook" \
      maintainer="pipelines-extcomm@redhat.com" \
      name="openshift-pipelines/pipelines-pruner-webhook-rhel9" \
      summary="Red Hat OpenShift Pipelines tektoncd-pruner webhook" \
      version="v1.18.0"

RUN groupadd -r -g 65532 nonroot && useradd --no-log-init -r -u 65532 -g nonroot nonroot
USER 65532

ENTRYPOINT $WEBHOOK