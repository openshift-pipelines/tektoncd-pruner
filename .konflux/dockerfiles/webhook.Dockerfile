ARG GO_BUILDER=registry.access.redhat.com/ubi9/go-toolset:latest@sha256:cab9089469c52029e4d8a4e9f717e87c51ab22eed448ebcada101b811f97dfe7
ARG UBI_RUNTIME=registry.redhat.io/ubi9/ubi-minimal-pqc:latest@sha256:72187ec58a94bd958ff48e41881d7c5d8bc0b49586cc1661da5ba4d92374c206

FROM $GO_BUILDER AS builder

WORKDIR /go/src/github.com/openshift-pipelines/tektoncd-pruner
COPY upstream .

ENV GODEBUG="http2server=0"
RUN go build -ldflags="-X 'knative.dev/pkg/changeset.rev=$(cat HEAD)'" -mod=vendor -tags disable_gcp -v -o /tmp/webhook \
    ./cmd/webhook

FROM $UBI_RUNTIME
ARG VERSION=next

ENV KO_APP=/ko-app \
    WEBHOOK=${KO_APP}/webhook

COPY --from=builder /tmp/webhook ${WEBHOOK}

LABEL \
    com.redhat.component="openshift-pipelines-pruner-webhook-rhel9-container" \
    cpe="cpe:/a:redhat:openshift_pipelines:next::el9" \
    description="Red Hat OpenShift Pipelines tektoncd-pruner webhook" \
    io.k8s.description="Red Hat OpenShift Pipelines tektoncd-pruner webhook" \
    io.k8s.display-name="Red Hat OpenShift Pipelines tektoncd-pruner webhook" \
    io.openshift.tags="tekton,openshift,tektoncd-pruner,webhook" \
    maintainer="pipelines-extcomm@redhat.com" \
    name="openshift-pipelines/pipelines-pruner-webhook-rhel9" \
    summary="Red Hat OpenShift Pipelines tektoncd-pruner webhook" \
    version="next"

RUN groupadd -r -g 65532 nonroot && useradd --no-log-init -r -u 65532 -g nonroot nonroot
USER 65532

ENTRYPOINT $WEBHOOK