ARG GO_BUILDER=registry.redhat.io/ubi9/go-toolset:latest@sha256:edb9452603fa48e9882063f001ecd7d322add20f1564a4f05f38f6683be5ea94
ARG UBI_RUNTIME=registry.access.redhat.com/ubi9/ubi-minimal@sha256:2d4c4551af6831dcd26bbf86108c4385de5183ec8fe6b5868f6f9640fe47f4ed

FROM $GO_BUILDER AS builder

WORKDIR /go/src/github.com/openshift-pipelines/tektoncd-pruner
COPY upstream .

ENV GODEBUG="http2server=0"
RUN go build -ldflags="-X 'knative.dev/pkg/changeset.rev=$(cat HEAD)'" -mod=vendor -tags disable_gcp -v -o /tmp/webhook \
    ./cmd/webhook

FROM $UBI_RUNTIME
ARG VERSION=1.24

ENV KO_APP=/ko-app \
    WEBHOOK=${KO_APP}/webhook

COPY --from=builder /tmp/webhook ${WEBHOOK}

LABEL \
    com.redhat.component="openshift-pipelines-pruner-webhook-rhel9-container" \
    cpe="cpe:/a:redhat:openshift_pipelines:1.24::el9" \
    description="Red Hat OpenShift Pipelines tektoncd-pruner webhook" \
    io.k8s.description="Red Hat OpenShift Pipelines tektoncd-pruner webhook" \
    io.k8s.display-name="Red Hat OpenShift Pipelines tektoncd-pruner webhook" \
    io.openshift.tags="tekton,openshift,tektoncd-pruner,webhook" \
    maintainer="pipelines-extcomm@redhat.com" \
    name="openshift-pipelines/pipelines-pruner-webhook-rhel9" \
    summary="Red Hat OpenShift Pipelines tektoncd-pruner webhook" \
    version="v1.24.2"

RUN groupadd -r -g 65532 nonroot && useradd --no-log-init -r -u 65532 -g nonroot nonroot
USER 65532

ENTRYPOINT $WEBHOOK