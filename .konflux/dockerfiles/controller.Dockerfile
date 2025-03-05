ARG GO_BUILDER=brew.registry.redhat.io/rh-osbs/openshift-golang-builder:v1.23
ARG RUNTIME=registry.access.redhat.com/ubi9/ubi-minimal:latest@sha256:66b99214cb9733e77c4a12cc3e3cbbe76769a213f4e2767f170a4f0fdf9db490

FROM $GO_BUILDER AS builder

WORKDIR /go/src/github.com/openshift-pipelines/tektoncd-pruner
COPY . .

ENV GODEBUG="http2server=0"
RUN go build -ldflags="-X 'knative.dev/pkg/changeset.rev=$(cat HEAD)'" -mod=vendor -tags disable_gcp -v -o /tmp/controller \
    ./cmd/controller

FROM $RUNTIME
ARG VERSION=tektoncd-pruner-1-18

ENV KO_APP=/ko-app \
    CONTROLLER=${KO_APP}/controller

COPY --from=builder /tmp/controller ${CONTROLLER}

LABEL \
      com.redhat.component="openshift-pipelines-tektoncd-pruner-controller-rhel9-container" \
      name="openshift-pipelines/pipelines-tektoncd-pruner-controller-rhel9" \
      version=$VERSION \
      summary="Red Hat OpenShift Pipelines tektoncd-pruner Controller" \
      maintainer="pipelines-extcomm@redhat.com" \
      description="Red Hat OpenShift Pipelines tektoncd-pruner Controller" \
      io.k8s.display-name="Red Hat OpenShift Pipelines tektoncd-pruner Controller" \
      io.k8s.description="Red Hat OpenShift Pipelines tektoncd-pruner Controller" \
      io.openshift.tags="pipelines,tekton,openshift"

RUN groupadd -r -g 65532 nonroot && useradd --no-log-init -r -u 65532 -g nonroot nonroot
USER 65532

ENTRYPOINT $CONTROLLER
