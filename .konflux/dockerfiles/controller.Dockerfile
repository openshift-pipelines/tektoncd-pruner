ARG GO_BUILDER=registry.redhat.io/ubi9/go-toolset:latest@sha256:cab9089469c52029e4d8a4e9f717e87c51ab22eed448ebcada101b811f97dfe7
ARG UBI_RUNTIME=registry.access.redhat.com/ubi9/ubi-minimal:latest@sha256:bba2b4bbf0ed15cfe7dd72a1e17ae7d2015a141f306b304e1a2785c41400e39c

FROM $GO_BUILDER AS builder

WORKDIR /go/src/github.com/openshift-pipelines/tektoncd-pruner
COPY upstream .

ENV GODEBUG="http2server=0"
RUN go build -ldflags="-X 'knative.dev/pkg/changeset.rev=$(cat HEAD)'" -mod=vendor -tags disable_gcp -v -o /tmp/controller \
    ./cmd/controller

FROM $UBI_RUNTIME
ARG VERSION=1.24

ENV KO_APP=/ko-app \
    CONTROLLER=${KO_APP}/controller

COPY --from=builder /tmp/controller ${CONTROLLER}

LABEL \
    com.redhat.component="openshift-pipelines-pruner-controller-rhel9-container" \
    cpe="cpe:/a:redhat:openshift_pipelines:1.24::el9" \
    description="Red Hat OpenShift Pipelines tektoncd-pruner controller" \
    io.k8s.description="Red Hat OpenShift Pipelines tektoncd-pruner controller" \
    io.k8s.display-name="Red Hat OpenShift Pipelines tektoncd-pruner controller" \
    io.openshift.tags="tekton,openshift,tektoncd-pruner,controller" \
    maintainer="pipelines-extcomm@redhat.com" \
    name="openshift-pipelines/pipelines-pruner-controller-rhel9" \
    summary="Red Hat OpenShift Pipelines tektoncd-pruner controller" \
    version="v1.24.3"

RUN groupadd -r -g 65532 nonroot && useradd --no-log-init -r -u 65532 -g nonroot nonroot
USER 65532

ENTRYPOINT $CONTROLLER
