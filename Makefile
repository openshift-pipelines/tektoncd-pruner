MODULE   = $(shell env GO111MODULE=on $(GO) list -m)
DATE    ?= $(shell date +%FT%T%z)
VERSION ?= $(shell git describe --tags --always --dirty --match=v* 2> /dev/null || \
			cat $(CURDIR)/.version 2> /dev/null || echo v0)
PKGS     = $(or $(PKG),$(shell env GO111MODULE=on $(GO) list ./...))
TESTPKGS = $(shell env GO111MODULE=on $(GO) list -f \
			'{{ if or .TestGoFiles .XTestGoFiles }}{{ .ImportPath }}{{ end }}' \
			$(PKGS))
BIN      = $(CURDIR)/.bin

GOLANGCI_VERSION = v1.47.2

GO           = go
TIMEOUT_UNIT = 5m
TIMEOUT_E2E  = 20m
V = 0
Q = $(if $(filter 1,$V),,@)
M = $(shell printf "\033[34;1m🐱\033[0m")

export GO111MODULE=on

# -----------------------------------------------------------------------------
# Target platform namespace switch
# Use TARGET=openshift to deploy into openshift-pipelines
# Default is tekton-pipelines
# -----------------------------------------------------------------------------
TARGET ?=
NAMESPACE := tekton-pipelines
ifneq ($(filter openshift,$(TARGET)),)
  NAMESPACE := openshift-pipelines
endif

# Helper to rewrite namespace in resolved manifests
SED_NS = sed -e 's/namespace: tekton-pipelines/namespace: $(NAMESPACE)/g'

COMMANDS=$(patsubst cmd/%,%,$(wildcard cmd/*))
BINARIES=$(addprefix bin/,$(COMMANDS))

.PHONY: all
all: fmt $(BINARIES) | $(BIN) ; $(info $(M) building executable…) @ ## Build program binary

$(BIN):
	@mkdir -p $@
$(BIN)/%: | $(BIN) ; $(info $(M) building $(PACKAGE)…)
	$Q tmp=$$(mktemp -d); cd $$tmp; \
		env GO111MODULE=on GOPATH=$$tmp GOBIN=$(BIN) $(GO) install $(PACKAGE) \
		|| ret=$$?; \
		env GO111MODULE=on GOPATH=$$tmp GOBIN=$(BIN) $(GO) clean -modcache \
        || ret=$$?; \
		cd - ; \
	  		rm -rf $$tmp ; exit $$ret

FORCE:

bin/%: cmd/% FORCE
	$Q $(GO) build -mod=vendor $(LDFLAGS) -v -o $@ ./$<

KO = $(or ${KO_BIN},${KO_BIN},$(BIN)/ko)
$(BIN)/ko: PACKAGE=github.com/google/ko@latest

.PHONY: apply
apply: | $(KO) ; $(info $(M) ko resolve | $(NAMESPACE) apply …) @ ## Apply config to the current cluster (supports TARGET=openshift)
	$Q $(KO) resolve -R -f config | $(SED_NS) | kubectl apply -f -

.PHONY: resolve
resolve: | $(KO) ; $(info $(M) ko resolve -R -f config/ (namespace=$(NAMESPACE))) @ ## Resolve config (prints to stdout)
	$Q $(KO) resolve --push=false --oci-layout-path=$(BIN)/oci -R -f config | $(SED_NS)

.PHONY: generated
generated: | vendor ; $(info $(M) update generated files) ## Update generated files
	$Q ./hack/update-codegen.sh

.PHONY: vendor
vendor:
	$Q ./hack/update-deps.sh

# Misc

.PHONY: clean
clean: ; $(info $(M) cleaning…) @ ## Cleanup everything
	@rm -rf $(BIN)
	@rm -rf bin
	@rm -rf test/tests.* test/coverage.*

.PHONY: help
help:
	@grep -hE '^[ a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-17s\033[0m %s\n", $$1, $$2}'
	@echo ""
	@echo "🚀 For observability setup: make observability-help"
	@echo ""
	@echo "Namespace: $(NAMESPACE) (set TARGET=openshift to switch)"

.PHONY: version
version:

	@echo $(VERSION)

.PHONY: deploy_tekton
deploy_tekton: clean_tekton | ; $(info $(M) deploying tekton on local cluster …) @ ## Deploying tekton on local cluster
	-kubectl apply --filename https://storage.googleapis.com/tekton-releases/pipeline/latest/release.yaml
	-$(KO) resolve -R -f config | $(SED_NS) | kubectl apply -f -

.PHONY:  clean_tekton 
clean_tekton: | ; $(info $(M) deleting tekton from cluster …) @ ## Deleting tekton on local cluster
	-$(KO) resolve -R -f config | $(SED_NS) | kubectl delete -f - --ignore-not-found

# Prerequisite: docker [or] podman and kind
# this will deploy a local registry using docker and create a kind cluster
# configuring with the registry
# then does make apply to deploy the operator
# and show the location of kubeconfig at last
.PHONY: dev-setup
dev-setup: # setup kind with local registry for local development
	@cd ./hack/dev/kind/;./install.sh

#Release
RELEASE_VERSION=v0.0.0
RELEASE_DIR ?= /tmp/tektoncd-pruner-${RELEASE_VERSION}

.PHONY: github-release
github-release:
	./hack/release.sh ${RELEASE_VERSION}

# =============================================================================
# Observability Commands
# =============================================================================

.PHONY: observability-setup-simple observability-test observability-clean observability-local

observability-setup-simple: ## Setup Kind cluster with simple observability stack (basic Prometheus)
	@./hack/setup-observability-simple.sh

observability-local: ## Start local port forwards for observability endpoints
	@echo "🚀 Starting local port forwards..."
	@kubectl port-forward -n $(NAMESPACE) svc/tekton-pruner-controller 9090:9090 &
	@kubectl port-forward -n $(NAMESPACE) svc/prometheus-operated 9091:9090 &
	@kubectl port-forward -n observability-system svc/tekton-pruner-jaeger-query 16686:16686 &
	@echo "✅ Port forwards started:"
	@echo "   📊 Metrics: http://localhost:9090/metrics"
	@echo "   📈 Prometheus: http://localhost:9091"
	@echo "   🔍 Jaeger: http://localhost:16686"
	@echo ""
	@echo "💡 To stop port forwards: pkill -f 'port-forward'"

observability-help: ## Show observability setup help
	@echo "🚀 Tekton Pruner Observability Commands:"
	@echo ""
	@echo "  make observability-setup   - Setup Kind cluster with full observability stack"
	@echo "  make observability-local   - Start local port forwards for dashboards"
	@echo ""
	@echo "📊 Complete setup process:"
	@echo "  1. make observability-setup    # ~5-10 minutes"
	@echo "  3. make observability-local    # Start dashboards"
	@echo "  4. Open http://localhost:9090/metrics to see metrics"
	@echo "  5. Open http://localhost:9091 for Prometheus"
	@echo "  6. Open http://localhost:16686 for Jaeger tracing"

