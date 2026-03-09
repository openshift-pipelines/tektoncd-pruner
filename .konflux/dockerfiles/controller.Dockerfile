ARG GO_BUILDER=registry.access.redhat.com/ubi9/go-toolset:1.25
ARG RUNTIME=registry.access.redhat.com/ubi9/ubi-minimal:latest@sha256:6fc28bcb6776e387d7a35a2056d9d2b985dc4e26031e98a2bd35a7137cd6fd71 

FROM $GO_BUILDER AS builder

WORKDIR /go/src/github.com/openshift-pipelines/tektoncd-pruner
COPY upstream .

ENV GODEBUG="http2server=0"
RUN go build -ldflags="-X 'knative.dev/pkg/changeset.rev=$(cat HEAD)'" -mod=vendor -tags disable_gcp -v -o /tmp/controller \
    ./cmd/controller

FROM $RUNTIME
ARG VERSION=1.15

ENV KO_APP=/ko-app \
    CONTROLLER=${KO_APP}/controller

COPY --from=builder /tmp/controller ${CONTROLLER}

LABEL \
    com.redhat.component="openshift-pipelines-pruner-controller-rhel9-container" \
    cpe="cpe:/a:redhat:openshift_pipelines:1.15::el9" \
    description="Red Hat OpenShift Pipelines tektoncd-pruner controller" \
    io.k8s.description="Red Hat OpenShift Pipelines tektoncd-pruner controller" \
    io.k8s.display-name="Red Hat OpenShift Pipelines tektoncd-pruner controller" \
    io.openshift.tags="tekton,openshift,tektoncd-pruner,controller" \
    maintainer="pipelines-extcomm@redhat.com" \
    name="openshift-pipelines/pipelines-pruner-controller-rhel9" \
    summary="Red Hat OpenShift Pipelines tektoncd-pruner controller" \
    version="v1.15.5"

RUN groupadd -r -g 65532 nonroot && useradd --no-log-init -r -u 65532 -g nonroot nonroot
USER 65532

ENTRYPOINT $CONTROLLER
