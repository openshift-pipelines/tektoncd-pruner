ARG GO_BUILDER=registry.redhat.io/ubi9/go-toolset:latest@sha256:51eef2836e5223c41577757c30544fbfb3f0710f3436b70b82d2195e35fcb743
ARG UBI_RUNTIME=registry.redhat.io/ubi9/ubi-minimal-pqc:latest@sha256:5e9a0970f73f4800445c3ce0ca0d6b7ea1a8c620b81e735d16432db09d6787bc

FROM $GO_BUILDER AS builder

WORKDIR /go/src/github.com/openshift-pipelines/tektoncd-pruner
COPY upstream .

ENV GODEBUG="http2server=0"
RUN go build -ldflags="-X 'knative.dev/pkg/changeset.rev=$(cat HEAD)'" -mod=vendor -tags disable_gcp -v -o /tmp/webhook \
    ./cmd/webhook

FROM $UBI_RUNTIME
ARG VERSION=1.25

ENV KO_APP=/ko-app \
    WEBHOOK=${KO_APP}/webhook

COPY --from=builder /tmp/webhook ${WEBHOOK}

LABEL \
    com.redhat.component="openshift-pipelines-pruner-webhook-rhel9-container" \
    cpe="cpe:/a:redhat:openshift_pipelines:1.25::el9" \
    description="Red Hat OpenShift Pipelines tektoncd-pruner webhook" \
    io.k8s.description="Red Hat OpenShift Pipelines tektoncd-pruner webhook" \
    io.k8s.display-name="Red Hat OpenShift Pipelines tektoncd-pruner webhook" \
    io.openshift.tags="tekton,openshift,tektoncd-pruner,webhook" \
    maintainer="pipelines-extcomm@redhat.com" \
    name="openshift-pipelines/pipelines-pruner-webhook-rhel9" \
    summary="Red Hat OpenShift Pipelines tektoncd-pruner webhook" \
    version="v1.25.0"

RUN groupadd -r -g 65532 nonroot && useradd --no-log-init -r -u 65532 -g nonroot nonroot
USER 65532

ENTRYPOINT $WEBHOOK