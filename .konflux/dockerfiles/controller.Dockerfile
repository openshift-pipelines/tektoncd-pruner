ARG GO_BUILDER=registry.redhat.io/ubi9/go-toolset:latest@sha256:2cac9704c8182ea8cf34a3cb8db3870f993446a0eb98d4068ac26e19e1900f41
ARG UBI_RUNTIME=registry.redhat.io/ubi9/ubi-minimal-pqc:latest@sha256:5e9a0970f73f4800445c3ce0ca0d6b7ea1a8c620b81e735d16432db09d6787bc

FROM $GO_BUILDER AS builder

WORKDIR /go/src/github.com/openshift-pipelines/tektoncd-pruner
COPY upstream .

ENV GODEBUG="http2server=0"
RUN go build -ldflags="-X 'knative.dev/pkg/changeset.rev=$(cat HEAD)'" -mod=vendor -tags disable_gcp -v -o /tmp/controller \
    ./cmd/controller

FROM $UBI_RUNTIME
ARG VERSION=1.25

ENV KO_APP=/ko-app \
    CONTROLLER=${KO_APP}/controller

COPY --from=builder /tmp/controller ${CONTROLLER}

LABEL \
    com.redhat.component="openshift-pipelines-pruner-controller-rhel9-container" \
    cpe="cpe:/a:redhat:openshift_pipelines:1.25::el9" \
    description="Red Hat OpenShift Pipelines tektoncd-pruner controller" \
    io.k8s.description="Red Hat OpenShift Pipelines tektoncd-pruner controller" \
    io.k8s.display-name="Red Hat OpenShift Pipelines tektoncd-pruner controller" \
    io.openshift.tags="tekton,openshift,tektoncd-pruner,controller" \
    maintainer="pipelines-extcomm@redhat.com" \
    name="openshift-pipelines/pipelines-pruner-controller-rhel9" \
    summary="Red Hat OpenShift Pipelines tektoncd-pruner controller" \
    version="v1.25.0"

RUN groupadd -r -g 65532 nonroot && useradd --no-log-init -r -u 65532 -g nonroot nonroot
USER 65532

ENTRYPOINT $CONTROLLER
