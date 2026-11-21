ARG GO_BUILDER=registry.redhat.io/ubi9/go-toolset:latest@sha256:2cac9704c8182ea8cf34a3cb8db3870f993446a0eb98d4068ac26e19e1900f41
ARG UBI_RUNTIME=registry.access.redhat.com/ubi9/ubi-minimal:latest@sha256:08d4c70f1baf6537a0728656cf7942d561e74a35916509d205b6519861266352

FROM $GO_BUILDER AS builder

WORKDIR /go/src/github.com/openshift-pipelines/tektoncd-pruner
COPY upstream .

ENV GODEBUG="http2server=0"
RUN go build -ldflags="-X 'knative.dev/pkg/changeset.rev=$(cat HEAD)'" -mod=vendor -tags disable_gcp -v -o /tmp/webhook \
    ./cmd/webhook

FROM $UBI_RUNTIME
ARG VERSION=1.23

ENV KO_APP=/ko-app \
    WEBHOOK=${KO_APP}/webhook

COPY --from=builder /tmp/webhook ${WEBHOOK}

LABEL \
    com.redhat.component="openshift-pipelines-pruner-webhook-rhel9-container" \
    cpe="cpe:/a:redhat:openshift_pipelines:1.23::el9" \
    description="Red Hat OpenShift Pipelines tektoncd-pruner webhook" \
    io.k8s.description="Red Hat OpenShift Pipelines tektoncd-pruner webhook" \
    io.k8s.display-name="Red Hat OpenShift Pipelines tektoncd-pruner webhook" \
    io.openshift.tags="tekton,openshift,tektoncd-pruner,webhook" \
    maintainer="pipelines-extcomm@redhat.com" \
    name="openshift-pipelines/pipelines-pruner-webhook-rhel9" \
    summary="Red Hat OpenShift Pipelines tektoncd-pruner webhook" \
    version="v1.23.5"

RUN groupadd -r -g 65532 nonroot && useradd --no-log-init -r -u 65532 -g nonroot nonroot
USER 65532

ENTRYPOINT $WEBHOOK