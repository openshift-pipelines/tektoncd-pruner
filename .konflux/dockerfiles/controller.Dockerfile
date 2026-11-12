ARG GO_BUILDER=registry.redhat.io/ubi9/go-toolset:latest@sha256:a2ad457dc7525635613d2355037239ca9bf4374ba3b59961d1cdaa4d5f1af7d8
ARG UBI_RUNTIME=registry.redhat.io/ubi9/ubi-minimal-pqc:latest@sha256:eb807555968d40a35c815329eec6ef52b56a8bb3e6f06c25f7440ead905f8c74

FROM $GO_BUILDER AS builder

WORKDIR /go/src/github.com/openshift-pipelines/tektoncd-pruner
COPY upstream .

ENV GODEBUG="http2server=0"
RUN go build -ldflags="-X 'knative.dev/pkg/changeset.rev=$(cat HEAD)'" -mod=vendor -tags disable_gcp -v -o /tmp/controller \
    ./cmd/controller

FROM $UBI_RUNTIME
ARG VERSION=next

ENV KO_APP=/ko-app \
    CONTROLLER=${KO_APP}/controller

COPY --from=builder /tmp/controller ${CONTROLLER}

LABEL \
    com.redhat.component="openshift-pipelines-pruner-controller-rhel9-container" \
    cpe="cpe:/a:redhat:openshift_pipelines:next::el9" \
    description="Red Hat OpenShift Pipelines tektoncd-pruner controller" \
    io.k8s.description="Red Hat OpenShift Pipelines tektoncd-pruner controller" \
    io.k8s.display-name="Red Hat OpenShift Pipelines tektoncd-pruner controller" \
    io.openshift.tags="tekton,openshift,tektoncd-pruner,controller" \
    maintainer="pipelines-extcomm@redhat.com" \
    name="openshift-pipelines/pipelines-pruner-controller-rhel9" \
    summary="Red Hat OpenShift Pipelines tektoncd-pruner controller" \
    version="next"

RUN groupadd -r -g 65532 nonroot && useradd --no-log-init -r -u 65532 -g nonroot nonroot
USER 65532

ENTRYPOINT $CONTROLLER
